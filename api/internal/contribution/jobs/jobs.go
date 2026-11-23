// Package jobs é o adaptador River do contribution: a limpeza periódica do
// rate limit. Os casos de uso (contribution/app) não conhecem o River.
package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
)

// PurgeInterval é de quanto em quanto tempo a limpeza roda. A chave do dia
// some até PurgeInterval depois de o dia acabar (CONTEXT.md: o IP "some em
// 24h"); a consulta é barata.
const PurgeInterval = 15 * time.Minute

// PurgeRateLimitsArgs apaga a chave do dia anterior e os contadores de
// horas passadas.
type PurgeRateLimitsArgs struct{}

func (PurgeRateLimitsArgs) Kind() string { return "contribution.purge_rate_limits" }

func (PurgeRateLimitsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
			rivertype.JobStateRunning, rivertype.JobStateScheduled,
		}},
	}
}

// PurgeRateLimitsWorker executa PurgeRateLimitsArgs.
type PurgeRateLimitsWorker struct {
	river.WorkerDefaults[PurgeRateLimitsArgs]
	uc *app.PurgeRateLimits
}

func NewPurgeRateLimitsWorker(uc *app.PurgeRateLimits) *PurgeRateLimitsWorker {
	return &PurgeRateLimitsWorker{uc: uc}
}

func (w *PurgeRateLimitsWorker) Timeout(*river.Job[PurgeRateLimitsArgs]) time.Duration {
	return time.Minute
}

func (w *PurgeRateLimitsWorker) Work(ctx context.Context, _ *river.Job[PurgeRateLimitsArgs]) error {
	return w.uc.Execute(ctx)
}

// Register põe os workers do contribution em workers.
func Register(workers *river.Workers, purge *PurgeRateLimitsWorker) error {
	return river.AddWorkerSafely(workers, purge)
}

// PeriodicJobs é a limpeza do rate limit. Roda também ao subir o worker:
// com deploys frequentes, o relógio do River reiniciaria antes de disparar.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(PurgeInterval),
			func() (river.JobArgs, *river.InsertOpts) { return PurgeRateLimitsArgs{}, nil },
			&river.PeriodicJobOpts{ID: PurgeRateLimitsArgs{}.Kind(), RunOnStart: true},
		),
	}
}
