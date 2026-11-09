// Comando nelisa: um binário, vários modos (docs/arquitetura.md).
//
//	nelisa api          servidor HTTP
//	nelisa worker       jobs em segundo plano (River, a partir da fase 1)
//	nelisa migrate      aplica as migrations e sai
//	nelisa healthcheck  checa a API local; usado pelo healthcheck do container
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	ingestapp "github.com/mphennrichs/nelisa/api/internal/ingest/app"
	ingestjobs "github.com/mphennrichs/nelisa/api/internal/ingest/jobs"
	"github.com/mphennrichs/nelisa/api/internal/platform/buildinfo"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/config"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/platform/jobs"
	"github.com/mphennrichs/nelisa/api/internal/platform/migrate"
	"github.com/mphennrichs/nelisa/api/internal/platform/observability"
	propapp "github.com/mphennrichs/nelisa/api/internal/proposition/app"
	propjobs "github.com/mphennrichs/nelisa/api/internal/proposition/jobs"
	"github.com/mphennrichs/nelisa/api/internal/proposition/officialtext"
	"github.com/mphennrichs/nelisa/api/migrations"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: nelisa <api|worker|migrate|healthcheck>")
		os.Exit(2)
	}
	if os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := observability.NewLogger()
	logger.Info("starting nelisa", "mode", os.Args[1],
		"version", buildinfo.Version, "commit", buildinfo.Commit)

	var err error
	switch os.Args[1] {
	case "api":
		err = runAPI(ctx, logger)
	case "worker":
		err = runWorker(ctx, logger)
	case "migrate":
		err = runMigrate(ctx, logger)
	default:
		err = fmt.Errorf("modo desconhecido: %q", os.Args[1])
	}
	if err != nil {
		logger.Error("exiting with error", "error", err.Error())
		os.Exit(1)
	}
}

func runMigrate(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv, false)
	if err != nil {
		return err
	}
	if err := migrate.Up(ctx, cfg.DatabaseURL, migrations.FS); err != nil {
		return err
	}
	logger.Info("migrations applied")
	return nil
}

func runAPI(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv, true)
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	clk := clock.System{}
	sources, err := newSources()
	if err != nil {
		return err
	}
	// A API só enfileira (confirmar um Vínculo agenda a Sincronização dele);
	// quem executa é o worker.
	riverClient, err := jobs.NewInsertOnlyClient(pool, logger)
	if err != nil {
		return err
	}
	sched := newRiverSchedulers(jobs.NewInserter(riverClient))
	content := newContentModule(newContentUseCases(postgresContentPorts(pool, clk, sched, sources), clk), contributionOptions(cfg), logger)
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           newRouter(cfg, pool, newTeamModule(cfg, postgresTeamPorts(pool, clk), clk, logger), content, logger),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	logger.Info("api listening", "addr", cfg.ListenAddr)

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// runWorker executa os jobs do River: a Sincronização de cada Vínculo de
// fonte confirmado e a rodada periódica que a enfileira para todos
// (SYNC_INTERVAL).
func runWorker(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv, false)
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	sources, err := newSources()
	if err != nil {
		return err
	}
	// O PDF oficial (Estruturação) é lido com o PDFium em WebAssembly; o
	// runtime só sobe na primeira leitura.
	pdf := officialtext.NewPDFium()
	defer pdf.Close()
	client, err := newWorkerClient(pool, sources, officialtext.New(pdf, nil), cfg.SyncInterval, clock.System{}, logger)
	if err != nil {
		return err
	}
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("iniciando o river: %w", err)
	}
	logger.Info("worker started", "sync_interval", cfg.SyncInterval.String(), "concurrency", jobs.DefaultConcurrency)
	<-ctx.Done()
	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return client.Stop(stopCtx)
}

// newWorkerClient monta o River do modo worker. A rodada periódica
// enfileira um job por Vínculo por um cliente insert-only à parte: os
// workers precisam estar registrados antes de o cliente que os executa
// nascer.
func newWorkerClient(pool *pgxpool.Pool, sources ingestapp.Sources, text propapp.OfficialText, interval time.Duration, clk clock.Clock, logger *slog.Logger) (*jobs.Client, error) {
	inserter, err := jobs.NewInsertOnlyClient(pool, logger)
	if err != nil {
		return nil, err
	}
	sched := newRiverSchedulers(jobs.NewInserter(inserter))
	ports := postgresContentPorts(pool, clk, sched, sources)
	uc := newContentUseCases(ports, clk)
	workers := river.NewWorkers()
	syncLink, syncAll := newIngestWorkers(uc, logger)
	if err := ingestjobs.Register(workers, syncLink, syncAll); err != nil {
		return nil, fmt.Errorf("registrando workers: %w", err)
	}
	run := propapp.NewRunStructuring(ports.versions, ports.structurings, text, ports.tx, clk)
	if err := propjobs.Register(workers, propjobs.NewStructureVersionWorker(run, logger)); err != nil {
		return nil, fmt.Errorf("registrando workers: %w", err)
	}
	return jobs.NewWorkerClient(pool, jobs.WorkerOptions{
		Workers: workers, PeriodicJobs: ingestjobs.PeriodicJobs(interval), Logger: logger,
	})
}

func healthcheck() int {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + addr + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
