package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	auditapp "github.com/mphennrichs/nelisa/api/internal/audit/app"
	auditstore "github.com/mphennrichs/nelisa/api/internal/audit/store"
	ingestapp "github.com/mphennrichs/nelisa/api/internal/ingest/app"
	ingesthttp "github.com/mphennrichs/nelisa/api/internal/ingest/httpapi"
	ingestjobs "github.com/mphennrichs/nelisa/api/internal/ingest/jobs"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/platform/jobs"
	progressapp "github.com/mphennrichs/nelisa/api/internal/progress/app"
	progressdomain "github.com/mphennrichs/nelisa/api/internal/progress/domain"
	progressstore "github.com/mphennrichs/nelisa/api/internal/progress/store"
	propapp "github.com/mphennrichs/nelisa/api/internal/proposition/app"
	propdomain "github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	prophttp "github.com/mphennrichs/nelisa/api/internal/proposition/httpapi"
	propjobs "github.com/mphennrichs/nelisa/api/internal/proposition/jobs"
	propstore "github.com/mphennrichs/nelisa/api/internal/proposition/store"
	queueapp "github.com/mphennrichs/nelisa/api/internal/queue/app"
	queuehttp "github.com/mphennrichs/nelisa/api/internal/queue/httpapi"
	queuestore "github.com/mphennrichs/nelisa/api/internal/queue/store"
	reviewapp "github.com/mphennrichs/nelisa/api/internal/review/app"
	reviewhttp "github.com/mphennrichs/nelisa/api/internal/review/httpapi"
	reviewstore "github.com/mphennrichs/nelisa/api/internal/review/store"
	teamapp "github.com/mphennrichs/nelisa/api/internal/team/app"
	teamstore "github.com/mphennrichs/nelisa/api/internal/team/store"
)

// Os módulos de conteúdo (proposition, progress, ingest) não se importam:
// cada um declara portas e este arquivo as liga. É o único lugar que
// conhece os três.

// txRunner é a porta de transação, igual em todos os módulos.
type txRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// scheduler enfileira jobs: Sincronizações (ingest/jobs.Scheduler) e
// Estruturações (proposition/jobs.Scheduler) em produção.
type scheduler interface {
	ScheduleSync(ctx context.Context, linkID string) error
	ScheduleSyncs(ctx context.Context, linkIDs []string) error
	ScheduleStructuring(ctx context.Context, versionID string) error
}

// riverSchedulers junta os agendadores do River dos módulos.
type riverSchedulers struct {
	sync        ingestjobs.Scheduler
	structuring propjobs.Scheduler
}

func newRiverSchedulers(ins jobs.Inserter) riverSchedulers {
	return riverSchedulers{sync: ingestjobs.NewScheduler(ins), structuring: propjobs.NewScheduler(ins)}
}

func (s riverSchedulers) ScheduleSync(ctx context.Context, linkID string) error {
	return s.sync.ScheduleSync(ctx, linkID)
}

func (s riverSchedulers) ScheduleSyncs(ctx context.Context, linkIDs []string) error {
	return s.sync.ScheduleSyncs(ctx, linkIDs)
}

func (s riverSchedulers) ScheduleStructuring(ctx context.Context, versionID string) error {
	return s.structuring.ScheduleStructuring(ctx, versionID)
}

// recordAudit grava um evento de Auditoria na transação do contexto.
type recordAudit func(ctx context.Context, in auditapp.RecordEventInput) error

// contentPorts são as implementações que os módulos de conteúdo recebem.
// Em produção vêm do Postgres e do River; nos testes, dos fakes.
type contentPorts struct {
	props    propapp.PropositionRepo
	versions propapp.VersionRepo
	contents propapp.ContentRepo
	topics   propapp.TopicRepo
	// structurings: Estruturação e Dispositivos de cada Versão.
	structurings propapp.StructuringRepo
	comments     propapp.CommentRepo
	links        propapp.LinkRepo
	approvals    reviewapp.ApprovalRepo
	fixes        reviewapp.MinorFixRepo
	errata       reviewapp.ErratumRepo
	claims       queueapp.ClaimRepo
	// users é o cadastro da equipe, só para nomes e conta ativa
	// (teamapp.Directory).
	users     teamapp.UserRepo
	entries   progressapp.EntryRepo
	tx        txRunner
	audit     recordAudit
	scheduler scheduler
	sources   ingestapp.Sources
}

func postgresContentPorts(pool *pgxpool.Pool, clk clock.Clock, sched scheduler, sources ingestapp.Sources) contentPorts {
	return contentPorts{
		props:        propstore.NewPropositions(pool),
		versions:     propstore.NewVersions(pool),
		contents:     propstore.NewContents(pool),
		topics:       propstore.NewTopics(pool),
		structurings: propstore.NewStructurings(pool),
		comments:     propstore.NewComments(pool),
		links:        propstore.NewLinks(pool),
		approvals:    reviewstore.New(pool),
		fixes:        reviewstore.NewMinorFixes(pool),
		errata:       reviewstore.NewErrata(pool),
		claims:       queuestore.New(pool),
		users:        teamstore.NewUsers(pool),
		entries:      progressstore.New(pool),
		tx:           db.NewTxRunner(pool),
		audit:        auditapp.NewRecordEvent(auditstore.New(pool), clk).Execute,
		scheduler:    sched,
		sources:      sources,
	}
}

// contentUseCases são os casos de uso montados, para a API e o worker.
type contentUseCases struct {
	prop     prophttp.UseCases
	review   reviewhttp.UseCases
	queue    queuehttp.UseCases
	ingest   ingesthttp.UseCases
	syncLink *ingestapp.SyncLink
	syncAll  *ingestapp.SyncAll
}

func newContentUseCases(p contentPorts, clk clock.Clock) contentUseCases {
	timeline := progressapp.NewTimeline(p.entries)
	repos := propapp.EditorialRepos{
		Props: p.props, Versions: p.versions, Contents: p.contents, Topics: p.topics, Structurings: p.structurings,
		Comments: p.comments,
	}
	catalog := propapp.NewCatalog(repos, p.links, p.scheduler, clk)
	ic := ingestCatalog{c: catalog}
	pw := progressWriter{uc: progressapp.NewRecordEntries(p.entries, clk)}
	tl := timelineReader{uc: timeline}
	pa := propositionAudit{record: p.audit}
	ia := ingestAudit{record: p.audit}

	// Ciclo editorial: proposition, review e queue não se importam; os
	// adaptadores de wire_editorial.go ligam as portas de um às operações
	// de sistema do outro.
	directory := teamapp.NewDirectory(p.users)
	editorial := propapp.NewEditorial(repos, clk)
	gate := queueapp.NewGate(p.claims, queuePeople{d: directory}, clk)
	rc := reviewCorrections{e: editorial}
	reports := reviewapp.NewReports(p.approvals, p.fixes, p.errata, reviewEditorial{e: editorial}, rc)
	corrections := reviewapp.CorrectionDeps{
		Fixes: p.fixes, Errata: p.errata, Content: rc, Claims: reviewClaims{g: gate},
		Tx: p.tx, Audit: reviewAudit{record: p.audit}, Clock: clk,
	}
	pc := propClaims{g: gate}
	se := propapp.StructuringEdit{Repos: repos, Claims: pc, Sched: p.scheduler, Tx: p.tx, Audit: pa, Clock: clk}
	pr := propReviews{r: reports}
	re := reviewEditorial{e: editorial}
	qp := queuePeople{d: directory}
	return contentUseCases{
		prop: prophttp.UseCases{
			ListPropositions:     propapp.NewListPropositions(p.props, p.links),
			GetProposition:       propapp.NewGetProposition(repos, p.links, tl, pc, pr, propPeople{d: directory}),
			ConfirmLink:          propapp.NewConfirmLink(p.links, p.tx, pa, p.scheduler, clk),
			RejectLink:           propapp.NewRejectLink(p.links, p.tx, pa),
			SubmitForReview:      propapp.NewSubmitForReview(p.props, p.tx, pa, clk),
			CreateVersion:        propapp.NewCreateVersion(repos, pc, p.scheduler, p.tx, pa, clk),
			UpdateContent:        propapp.NewUpdateContent(repos, pc, p.tx, pa, clk),
			ConfirmContent:       propapp.NewConfirmContent(repos, pc, p.tx, pa, clk),
			ReprocessStructuring: propapp.NewReprocessStructuring(se),
			ConfirmStructuring:   propapp.NewConfirmStructuring(se),
			RetryStructuring:     propapp.NewRetryStructuring(se),
			CreateComment:        propapp.NewCreateComment(se),
			UpdateComment:        propapp.NewUpdateComment(se),
			DeleteComment:        propapp.NewDeleteComment(se),
			ConfirmComment:       propapp.NewConfirmComment(se),
			ListTopics:           propapp.NewListTopics(p.topics),
			CreateTopic:          propapp.NewCreateTopic(p.topics, p.tx, pa, clk),
			UpdateTopic:          propapp.NewUpdateTopic(p.topics, p.tx, pa),
			ListPublic:           propapp.NewListPublicPropositions(repos, p.links, pr),
			GetPublic:            propapp.NewGetPublicProposition(repos, p.links, tl, pr),
		},
		review: reviewhttp.UseCases{
			Approve:      reviewapp.NewApprove(p.approvals, re, reviewClaims{g: gate}, p.tx, reviewAudit{record: p.audit}, clk),
			SecondReview: reviewapp.NewSecondReview(p.approvals, re, p.tx, reviewAudit{record: p.audit}, clk),
			// Correção menor e Errata: o review orquestra (regras,
			// Auditoria, transação); o conteúdo continua gravado pelo
			// proposition, pela porta Corrections.
			ApplyMinorFix:   reviewapp.NewApplyMinorFix(corrections),
			CheckMinorFix:   reviewapp.NewCheckMinorFix(corrections),
			ProposeErratum:  reviewapp.NewProposeErratum(corrections),
			ApproveErratum:  reviewapp.NewApproveErratum(corrections),
			RejectErratum:   reviewapp.NewRejectErratum(corrections),
			WithdrawErratum: reviewapp.NewWithdrawErratum(corrections),
		},
		queue: queuehttp.UseCases{
			Claim:     queueapp.NewClaim(p.claims, queueProps{e: editorial}, qp, p.tx, clk),
			Release:   queueapp.NewRelease(p.claims, qp, p.tx, queueAudit{record: p.audit}, clk),
			ListQueue: queueapp.NewListQueue(queueProps{e: editorial}, queueReviewWork{r: reports}, p.claims, qp, clk),
		},
		ingest: ingesthttp.UseCases{
			SearchSources: ingestapp.NewSearchSources(p.sources, ic),
			Import:        ingestapp.NewImport(p.sources, ic, pw, p.tx, ia),
		},
		syncLink: ingestapp.NewSyncLink(p.sources, ic, pw, p.tx),
		syncAll:  ingestapp.NewSyncAll(ic, p.scheduler),
	}
}

// contentModule é o que o roteador precisa dos módulos de conteúdo.
type contentModule struct {
	prop   *prophttp.Handlers
	review *reviewhttp.Handlers
	queue  *queuehttp.Handlers
	ingest *ingesthttp.Handlers
}

func newContentModule(uc contentUseCases, logger *slog.Logger) contentModule {
	return contentModule{
		prop: prophttp.New(uc.prop, logger), review: reviewhttp.New(uc.review, logger),
		queue: queuehttp.New(uc.queue, logger), ingest: ingesthttp.New(uc.ingest, logger),
	}
}

// newIngestWorkers monta os workers do River para o modo worker.
func newIngestWorkers(uc contentUseCases, logger *slog.Logger) (*ingestjobs.SyncLinkWorker, *ingestjobs.SyncAllWorker) {
	return ingestjobs.NewSyncLinkWorker(uc.syncLink, logger), ingestjobs.NewSyncAllWorker(uc.syncAll, logger)
}

// --- Auditoria ---

type propositionAudit struct{ record recordAudit }

func (a propositionAudit) Record(ctx context.Context, e propapp.AuditEvent) error {
	return a.record(ctx, auditapp.RecordEventInput{
		ActorID: e.ActorID, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID, Metadata: e.Metadata,
	})
}

type ingestAudit struct{ record recordAudit }

func (a ingestAudit) Record(ctx context.Context, e ingestapp.AuditEvent) error {
	return a.record(ctx, auditapp.RecordEventInput{
		ActorID: e.ActorID, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID, Metadata: e.Metadata,
	})
}

// --- Tramitação ---

// timelineReader liga a porta TimelineReader do proposition ao progress.
type timelineReader struct{ uc *progressapp.Timeline }

func (t timelineReader) Timeline(ctx context.Context, propositionID string) ([]propapp.TimelineEntry, error) {
	es, err := t.uc.Execute(ctx, propositionID)
	if err != nil {
		return nil, err
	}
	out := make([]propapp.TimelineEntry, 0, len(es))
	for _, e := range es {
		out = append(out, propapp.TimelineEntry{
			Chamber: e.Chamber, At: e.At, Body: e.Body, Description: e.Description,
			Situation: e.Situation, Dispatch: e.Dispatch,
		})
	}
	return out, nil
}

// progressWriter liga a porta ProgressWriter do ingest ao progress.
type progressWriter struct{ uc *progressapp.RecordEntries }

func (p progressWriter) Record(ctx context.Context, propositionID string, chamber ingestapp.ChamberCode, entries []ingestapp.SourceProgress) (int, error) {
	in := progressapp.RecordEntriesInput{PropositionID: propositionID, Chamber: string(chamber)}
	for _, e := range entries {
		in.Entries = append(in.Entries, progressapp.EntryInput{
			ExternalKey: e.ExternalKey, At: e.At, Body: e.Body, Description: e.Description,
			Situation: e.Situation, Dispatch: e.Dispatch,
		})
	}
	res, err := p.uc.Execute(ctx, in)
	if err != nil {
		// Tramitação que não passa na validação é resposta inválida da
		// fonte: para o ingest, fonte indisponível (tenta de novo depois).
		if isProgressValidation(err) {
			return 0, errors.Join(ingestapp.ErrInvalidRecord, ingestapp.ErrUnavailable, err)
		}
		return 0, err
	}
	return res.Inserted, nil
}

func isProgressValidation(err error) bool {
	return errors.Is(err, progressapp.ErrTooManyEntries) || errors.Is(err, progressdomain.ErrInvalidEntry)
}

// --- Catálogo de Proposições para o ingest ---

// ingestCatalog liga a porta Catalog do ingest ao proposition, traduzindo
// tipos e erros.
type ingestCatalog struct{ c *propapp.Catalog }

func toLinkRef(l propdomain.SourceLink) ingestapp.LinkRef {
	return ingestapp.LinkRef{
		ID: l.ID, PropositionID: l.PropositionID, Chamber: ingestapp.ChamberCode(l.Chamber),
		ExternalID: l.ExternalID, Confirmed: l.State == propdomain.LinkConfirmed,
	}
}

// catalogErr traduz erros do proposition para os do ingest.
func catalogErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, propdomain.ErrLinkNotFound), errors.Is(err, propdomain.ErrPropositionNotFound):
		return errors.Join(ingestapp.ErrLinkNotFound, err)
	case errors.Is(err, propdomain.ErrLinkTaken):
		return errors.Join(ingestapp.ErrLinkTaken, err)
	case errors.Is(err, propdomain.ErrPropositionExists), errors.Is(err, propdomain.ErrSlugTaken),
		errors.Is(err, propdomain.ErrChamberAlreadyLinked):
		return errors.Join(ingestapp.ErrPropositionExists, err)
	case errors.Is(err, propdomain.ErrInvalidKind), errors.Is(err, propdomain.ErrInvalidNumber),
		errors.Is(err, propdomain.ErrInvalidYear), errors.Is(err, propdomain.ErrInvalidChamber),
		errors.Is(err, propdomain.ErrInvalidSphere), errors.Is(err, propdomain.ErrInvalidExternal):
		// Registro que não passa na validação do domínio: resposta inválida
		// da fonte.
		return errors.Join(ingestapp.ErrInvalidRecord, ingestapp.ErrUnavailable, err)
	}
	return err
}

func officialData(r ingestapp.SourceRecord) propdomain.OfficialData {
	d := propdomain.OfficialData{
		OfficialSummary: r.OfficialSummary, OfficialTextURL: r.OfficialTextURL, Authors: r.Authors,
		Status: r.Status, AttachedToName: r.AttachedTo,
	}
	if !r.PresentedAt.IsZero() {
		t := r.PresentedAt
		d.PresentedAt = &t
	}
	return d
}

func sourceName(r ingestapp.SourceRecord) string {
	if r.Kind == "" {
		return ""
	}
	k, err := propdomain.ParseKind(r.Kind)
	if err != nil {
		return ""
	}
	return propdomain.OfficialName(k, r.Number, r.Year)
}

func (ic ingestCatalog) LinksByExternal(ctx context.Context, chamber ingestapp.ChamberCode, ids []string) ([]ingestapp.LinkRef, error) {
	ls, err := ic.c.LinksByExternal(ctx, propdomain.Chamber(chamber), ids)
	if err != nil {
		return nil, catalogErr(err)
	}
	out := make([]ingestapp.LinkRef, 0, len(ls))
	for _, l := range ls {
		out = append(out, toLinkRef(l))
	}
	return out, nil
}

func (ic ingestCatalog) CreateImported(ctx context.Context, in ingestapp.ImportRequest) (ingestapp.Imported, error) {
	r := in.Record
	req := propapp.CreateImportedInput{
		Chamber: propdomain.Chamber(r.Chamber), ExternalID: r.ExternalID,
		SourceName: sourceName(r), SourceURL: r.SourceURL,
		Kind: r.Kind, Number: r.Number, Year: r.Year, OriginChamber: propdomain.Chamber(r.OriginChamber),
		Data: officialData(r), LatestEventAt: in.LatestEventAt, EditorID: in.EditorID,
	}
	for _, c := range in.Candidates {
		req.Candidates = append(req.Candidates, propapp.Candidate{
			Chamber: propdomain.Chamber(c.Chamber), ExternalID: c.ExternalID, SourceName: c.SourceName, SourceURL: c.SourceURL,
		})
	}
	res, err := ic.c.CreateImported(ctx, req)
	if err != nil {
		return ingestapp.Imported{}, catalogErr(err)
	}
	return ingestapp.Imported{PropositionID: res.Proposition.ID, Name: res.Proposition.Name(), Suggested: len(res.Suggested)}, nil
}

func (ic ingestCatalog) Link(ctx context.Context, id string) (ingestapp.LinkRef, error) {
	l, err := ic.c.Link(ctx, id)
	if err != nil {
		return ingestapp.LinkRef{}, catalogErr(err)
	}
	return toLinkRef(l), nil
}

func (ic ingestCatalog) ConfirmedLinkIDs(ctx context.Context) ([]string, error) {
	ids, err := ic.c.ConfirmedLinkIDs(ctx)
	return ids, catalogErr(err)
}

func (ic ingestCatalog) ApplySync(ctx context.Context, in ingestapp.SyncResult) error {
	return catalogErr(ic.c.ApplySync(ctx, propapp.ApplySyncInput{
		LinkID: in.LinkID, Data: officialData(in.Record), LatestEventAt: in.LatestEventAt,
		SourceName: sourceName(in.Record), SourceURL: in.Record.SourceURL,
		ReportsAttachment: ingestapp.ReportsAttachment(in.Record.Chamber),
	}))
}

func (ic ingestCatalog) RecordSyncFailure(ctx context.Context, linkID, msg string) error {
	return catalogErr(ic.c.RecordSyncFailure(ctx, linkID, msg))
}
