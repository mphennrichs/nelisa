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

	"github.com/mphennrichs/nelisa/api/internal/platform/buildinfo"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/config"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/platform/migrate"
	"github.com/mphennrichs/nelisa/api/internal/platform/observability"
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
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           newRouter(cfg, pool, newTeamModule(cfg, postgresTeamPorts(pool, clk), clk, logger), logger),
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

// runWorker ainda não tem jobs: a fila do River entra na fase 1 (Importação
// e Sincronização). Por enquanto só valida a config e espera o sinal de
// parada, para o container já existir no stack.
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
	logger.Info("worker idle: no jobs registered yet")
	<-ctx.Done()
	return nil
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
