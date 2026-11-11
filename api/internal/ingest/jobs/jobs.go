// Package jobs é o adaptador River do ingest: os jobs de Sincronização, os
// workers que os executam e o agendador que os enfileira. Os casos de uso
// (ingest/app) não conhecem o River.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
)

// maxAttempts com o recuo padrão do River (tentativa⁴ segundos) cobre
// algumas horas de fonte fora do ar antes de desistir; a Sincronização
// periódica seguinte tenta de novo de qualquer jeito.
const maxAttempts = 8

// syncTimeout limita um job: buscar registro e Tramitação de uma Casa.
const syncTimeout = 2 * time.Minute

// pendingStates: um Vínculo não ganha um segundo job enquanto o primeiro
// ainda não terminou. Concluídos ficam de fora para a próxima rodada
// periódica poder enfileirar de novo.
var pendingStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
	rivertype.JobStateRunning, rivertype.JobStateScheduled,
}

// SyncLinkArgs é a Sincronização de um Vínculo de fonte confirmado.
type SyncLinkArgs struct {
	LinkID string `json:"link_id"`
}

func (SyncLinkArgs) Kind() string { return "ingest.sync_link" }

func (SyncLinkArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: maxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: pendingStates},
	}
}

// SyncAllArgs é a rodada periódica: enfileira um SyncLinkArgs por Vínculo.
type SyncAllArgs struct{}

func (SyncAllArgs) Kind() string { return "ingest.sync_all" }

func (SyncAllArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: pendingStates},
	}
}

// SyncLinkWorker executa SyncLinkArgs.
type SyncLinkWorker struct {
	river.WorkerDefaults[SyncLinkArgs]
	uc     *app.SyncLink
	logger *slog.Logger
}

func NewSyncLinkWorker(uc *app.SyncLink, logger *slog.Logger) *SyncLinkWorker {
	return &SyncLinkWorker{uc: uc, logger: logger}
}

func (w *SyncLinkWorker) Timeout(*river.Job[SyncLinkArgs]) time.Duration { return syncTimeout }

func (w *SyncLinkWorker) Work(ctx context.Context, job *river.Job[SyncLinkArgs]) error {
	res, err := w.uc.Execute(ctx, job.Args.LinkID)
	switch {
	case errors.Is(err, app.ErrNotFound), errors.Is(err, app.ErrUnknownChamber):
		// Tentar de novo não muda nada; a falha já ficou anotada no Vínculo.
		return river.JobCancel(err)
	case err != nil:
		// ErrUnavailable e falhas de banco: o River tenta de novo, com recuo.
		return err
	}
	w.logger.InfoContext(ctx, "source link synced",
		"link_id", job.Args.LinkID, "skipped", res.Skipped, "new_progress", res.NewProgress)
	return nil
}

// SyncAllWorker executa SyncAllArgs.
type SyncAllWorker struct {
	river.WorkerDefaults[SyncAllArgs]
	uc     *app.SyncAll
	logger *slog.Logger
}

func NewSyncAllWorker(uc *app.SyncAll, logger *slog.Logger) *SyncAllWorker {
	return &SyncAllWorker{uc: uc, logger: logger}
}

func (w *SyncAllWorker) Work(ctx context.Context, _ *river.Job[SyncAllArgs]) error {
	n, err := w.uc.Execute(ctx)
	if err != nil {
		return err
	}
	w.logger.InfoContext(ctx, "sync scheduled for all confirmed links", "links", n)
	return nil
}

// RadarScanArgs é a rodada diária do Radar (#45).
type RadarScanArgs struct{}

func (RadarScanArgs) Kind() string { return "ingest.radar_scan" }

func (RadarScanArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 5,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: pendingStates},
	}
}

// radarTimeout: a pauta de uma semana nas duas Casas leva poucos minutos.
const radarTimeout = 10 * time.Minute

// RadarScanWorker executa RadarScanArgs.
type RadarScanWorker struct {
	river.WorkerDefaults[RadarScanArgs]
	uc     *app.ScanRadar
	logger *slog.Logger
}

func NewRadarScanWorker(uc *app.ScanRadar, logger *slog.Logger) *RadarScanWorker {
	return &RadarScanWorker{uc: uc, logger: logger}
}

func (w *RadarScanWorker) Timeout(*river.Job[RadarScanArgs]) time.Duration { return radarTimeout }

func (w *RadarScanWorker) Work(ctx context.Context, _ *river.Job[RadarScanArgs]) error {
	res, err := w.uc.Execute(ctx)
	w.logger.InfoContext(ctx, "radar scanned", "found", res.Found, "purged", res.Purged)
	// Uma Casa fora do ar: o que veio da outra já foi gravado; o River
	// tenta de novo com recuo.
	return err
}

// Register põe os workers do ingest em workers.
func Register(workers *river.Workers, syncLink *SyncLinkWorker, syncAll *SyncAllWorker, radar *RadarScanWorker) error {
	if err := river.AddWorkerSafely(workers, syncLink); err != nil {
		return err
	}
	if err := river.AddWorkerSafely(workers, syncAll); err != nil {
		return err
	}
	return river.AddWorkerSafely(workers, radar)
}

// dailyAt é o horário de Brasília da rodada do Radar: depois que as Casas
// publicam a pauta do dia e antes de a equipe começar.
type dailyAt struct {
	hour int
	loc  *time.Location
}

// Next implementa river.PeriodicSchedule.
func (d dailyAt) Next(t time.Time) time.Time {
	l := t.In(d.loc)
	n := time.Date(l.Year(), l.Month(), l.Day(), d.hour, 0, 0, 0, d.loc)
	if !n.After(t) {
		n = n.AddDate(0, 0, 1)
	}
	return n
}

// RadarHour é a hora (Brasília) da rodada diária do Radar.
const RadarHour = 6

// PeriodicJobs é a Sincronização periódica de todos os Vínculos. Roda
// também quando o worker sobe: com deploys mais frequentes que o
// intervalo, o relógio do River reiniciaria sem nunca disparar.
//
// O Radar roda uma vez por dia, às RadarHour de Brasília, e não ao subir:
// quem precisar dele na hora usa a rodada manual da tela.
func PeriodicJobs(interval time.Duration) []*river.PeriodicJob {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.FixedZone("BRT", -3*60*60)
	}
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(interval),
			func() (river.JobArgs, *river.InsertOpts) { return SyncAllArgs{}, nil },
			&river.PeriodicJobOpts{ID: SyncAllArgs{}.Kind(), RunOnStart: true},
		),
		river.NewPeriodicJob(
			dailyAt{hour: RadarHour, loc: loc},
			func() (river.JobArgs, *river.InsertOpts) { return RadarScanArgs{}, nil },
			&river.PeriodicJobOpts{ID: RadarScanArgs{}.Kind()},
		),
	}
}

// Inserter é o que o agendador precisa do River (platform/jobs.Inserter).
type Inserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
	InsertMany(ctx context.Context, params []river.InsertManyParams) error
}

// Scheduler enfileira Sincronizações. Implementa app.SyncScheduler do
// ingest e a porta SyncScheduler do proposition (confirmar um Vínculo
// enfileira a Sincronização dele na mesma transação).
type Scheduler struct{ ins Inserter }

func NewScheduler(ins Inserter) Scheduler { return Scheduler{ins: ins} }

func (s Scheduler) ScheduleSync(ctx context.Context, linkID string) error {
	return s.ins.Insert(ctx, SyncLinkArgs{LinkID: linkID}, nil)
}

func (s Scheduler) ScheduleSyncs(ctx context.Context, linkIDs []string) error {
	params := make([]river.InsertManyParams, 0, len(linkIDs))
	for _, id := range linkIDs {
		params = append(params, river.InsertManyParams{Args: SyncLinkArgs{LinkID: id}})
	}
	return s.ins.InsertMany(ctx, params)
}

// ScheduleRadarScan implementa app.RadarScheduler.
func (s Scheduler) ScheduleRadarScan(ctx context.Context) error {
	return s.ins.Insert(ctx, RadarScanArgs{}, nil)
}
