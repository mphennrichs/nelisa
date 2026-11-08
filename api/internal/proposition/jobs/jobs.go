// Package jobs é o adaptador River do proposition: a Estruturação (buscar
// e ler o PDF oficial de uma Versão). O caso de uso (app.RunStructuring)
// não conhece o River.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// maxAttempts com o recuo padrão do River (tentativa⁴ s) cobre uns dez
// minutos de fonte fora do ar; depois a Estruturação fica manual
// ("network") e o Editor pode pedir outra busca.
const maxAttempts = 5

// timeout de uma tentativa: baixar (até 60 s) e ler o PDF.
const timeout = 3 * time.Minute

var pendingStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
	rivertype.JobStateRunning, rivertype.JobStateScheduled,
}

// StructureVersionArgs é a Estruturação automática de uma Versão.
type StructureVersionArgs struct {
	VersionID string `json:"version_id"`
}

func (StructureVersionArgs) Kind() string { return "proposition.structure_version" }

func (StructureVersionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: maxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: pendingStates},
	}
}

// StructureVersionWorker executa StructureVersionArgs.
type StructureVersionWorker struct {
	river.WorkerDefaults[StructureVersionArgs]
	uc     *app.RunStructuring
	logger *slog.Logger
}

func NewStructureVersionWorker(uc *app.RunStructuring, logger *slog.Logger) *StructureVersionWorker {
	return &StructureVersionWorker{uc: uc, logger: logger}
}

func (w *StructureVersionWorker) Timeout(*river.Job[StructureVersionArgs]) time.Duration {
	return timeout
}

func (w *StructureVersionWorker) Work(ctx context.Context, job *river.Job[StructureVersionArgs]) error {
	res, err := w.uc.Execute(ctx, job.Args.VersionID, job.Attempt >= job.MaxAttempts)
	switch {
	case errors.Is(err, domain.ErrStructuringNotFound), errors.Is(err, domain.ErrVersionNotFound):
		return river.JobCancel(err) // a Versão sumiu: nada a fazer
	case err != nil:
		return err
	}
	w.logger.InfoContext(ctx, "version structured",
		"version_id", job.Args.VersionID, "state", res.State, "failure", res.Failure,
		"provisions", res.Provisions, "warnings", res.Warnings)
	return nil
}

// Register põe o worker do proposition em workers.
func Register(workers *river.Workers, w *StructureVersionWorker) error {
	return river.AddWorkerSafely(workers, w)
}

// Inserter é o que o agendador precisa do River (platform/jobs.Inserter).
type Inserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

// Scheduler implementa app.StructuringScheduler.
type Scheduler struct{ ins Inserter }

func NewScheduler(ins Inserter) Scheduler { return Scheduler{ins: ins} }

func (s Scheduler) ScheduleStructuring(ctx context.Context, versionID string) error {
	return s.ins.Insert(ctx, StructureVersionArgs{VersionID: versionID}, nil)
}
