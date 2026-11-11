package jobs_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/ingest/jobs"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	platformjobs "github.com/mphennrichs/nelisa/api/internal/platform/jobs"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
)

var t0 = time.Date(2019, 11, 26, 12, 0, 0, 0, time.UTC)

// setup importa a PEC 221/2019 pela Câmara (fonte falsa) e devolve o
// Vínculo confirmado, pronto para sincronizar.
func setup(t *testing.T) (*fakes.World, *fakes.Source, *app.SyncLink, string) {
	t.Helper()
	w := fakes.NewWorld()
	camara := fakes.NewSource(app.ChamberCamara)
	camara.Put(app.SourceRecord{ExternalID: "2192459", Kind: "PEC", Number: 221, Year: 2019},
		fakes.Progress1("1-100", t0, "Apresentação"))
	sources := app.NewSources(camara)
	imp := app.NewImport(sources, w.Catalog(), w.Progress(), w.Tx(), w.Audit())
	if _, err := imp.Execute(editorCtx(), app.ImportInput{Chamber: "camara", ExternalID: "2192459"}); err != nil {
		t.Fatal(err)
	}
	ids, _ := w.Catalog().ConfirmedLinkIDs(context.Background())
	return w, camara, app.NewSyncLink(sources, w.Catalog(), w.Progress(), w.Tx()), ids[0]
}

func TestSyncLinkWorkerProcessesJobAgainstFakeSource(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	w, camara, uc, linkID := setup(t)
	camara.Put(app.SourceRecord{ExternalID: "2192459", Kind: "PEC", Number: 221, Year: 2019, Status: "Remetida ao Senado"},
		fakes.Progress1("1-100", t0, "Apresentação"), fakes.Progress1("2-200", t0.Add(time.Hour), "Aprovada"))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := rivertest.NewWorker(t, riverpgxv5.New(nil), &river.Config{Schema: platformjobs.Schema},
		jobs.NewSyncLinkWorker(uc, logger))

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for range 2 { // duas vezes: a Sincronização é idempotente
		res, err := worker.Work(ctx, t, tx, jobs.SyncLinkArgs{LinkID: linkID}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.EventKind != river.EventKindJobCompleted {
			t.Fatalf("event = %s", res.EventKind)
		}
	}
	if w.ProgressCount() != 2 {
		t.Errorf("progress = %d, want 2", w.ProgressCount())
	}
	if l := w.LinkByID(linkID); l.Synced != 2 || l.LastError != "" {
		t.Errorf("link = %+v", l)
	}

	// Fonte fora do ar: o job falha e fica para nova tentativa.
	camara.FetchErr = fakes.Unavailable()
	res, err := worker.Work(ctx, t, tx, jobs.SyncLinkArgs{LinkID: linkID}, nil)
	if !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	// Primeira falha: volta para a fila (available ou retryable, conforme o
	// recuo), nunca descartada.
	if res.EventKind != river.EventKindJobFailed || (res.Job.State != rivertype.JobStateRetryable && res.Job.State != rivertype.JobStateAvailable) || len(res.Job.Errors) != 1 {
		t.Errorf("unavailable: event=%s state=%s", res.EventKind, res.Job.State)
	}
	if res.Job.MaxAttempts != 8 {
		t.Errorf("max attempts = %d", res.Job.MaxAttempts)
	}

	// A fonte não tem mais o registro: tentar de novo não adianta.
	camara.FetchErr = app.ErrNotFound
	res, err = worker.Work(ctx, t, tx, jobs.SyncLinkArgs{LinkID: linkID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.EventKind != river.EventKindJobCancelled {
		t.Errorf("not found: event=%s", res.EventKind)
	}
}

func TestSchedulerEnqueuesInsideTheTransaction(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	client, err := platformjobs.NewInsertOnlyClient(pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	sched := jobs.NewScheduler(platformjobs.NewInserter(client))
	driver := riverpgxv5.New(pool)
	opts := &rivertest.RequireInsertedOpts{Schema: platformjobs.Schema}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.InsertTx(ctx, tx, jobs.SyncLinkArgs{LinkID: "rolled-back"}, nil); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	rivertest.RequireNotInserted(ctx, t, driver, jobs.SyncLinkArgs{LinkID: "rolled-back"}, opts)

	if err := sched.ScheduleSyncs(ctx, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	// Um job pendente por Vínculo: agendar de novo não duplica.
	if err := sched.ScheduleSync(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	rivertest.RequireManyInserted(ctx, t, driver, []rivertest.ExpectedJob{
		{Args: jobs.SyncLinkArgs{LinkID: "a"}, Opts: opts},
		{Args: jobs.SyncLinkArgs{LinkID: "b"}, Opts: opts},
	})
}

func TestPeriodicJobsAreConfigured(t *testing.T) {
	// Sincronização a cada intervalo e Radar diário.
	if p := jobs.PeriodicJobs(6 * time.Hour); len(p) != 2 {
		t.Fatalf("periodic jobs = %d", len(p))
	}
}

func editorCtx() context.Context {
	return auth.WithActor(context.Background(), auth.Actor{UserID: "editor-1", Role: auth.RoleEditor})
}
