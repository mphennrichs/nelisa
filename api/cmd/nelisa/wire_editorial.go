package main

import (
	"context"
	"errors"

	auditapp "github.com/mphennrichs/nelisa/api/internal/audit/app"
	propapp "github.com/mphennrichs/nelisa/api/internal/proposition/app"
	propdomain "github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	queueapp "github.com/mphennrichs/nelisa/api/internal/queue/app"
	queuedomain "github.com/mphennrichs/nelisa/api/internal/queue/domain"
	reviewapp "github.com/mphennrichs/nelisa/api/internal/review/app"
	reviewdomain "github.com/mphennrichs/nelisa/api/internal/review/domain"
	teamapp "github.com/mphennrichs/nelisa/api/internal/team/app"
)

// Adaptadores do ciclo editorial: proposition (Versão, conteúdo), review
// (aprovação, Selo), queue (Posse, Fila) e team (nomes, conta ativa) não
// se importam. Cada um declara portas no próprio app/ e estes adaptadores
// as ligam, traduzindo tipos e erros.

// --- Posse para o proposition ---

type propClaims struct{ g *queueapp.Gate }

func (c propClaims) TouchForEdit(ctx context.Context, propositionID, userID string) error {
	err := c.g.TouchForEdit(ctx, propositionID, userID)
	var taken *queuedomain.ClaimTakenError
	if errors.As(err, &taken) {
		return &propapp.ClaimTakenError{HolderName: taken.HolderName}
	}
	if errors.Is(err, queueapp.ErrPropositionNotFound) {
		return errors.Join(propdomain.ErrPropositionNotFound, err)
	}
	return err
}

func (c propClaims) Current(ctx context.Context, propositionID string) (*propapp.ClaimInfo, error) {
	v, err := c.g.Current(ctx, propositionID)
	if err != nil || v == nil {
		return nil, err
	}
	return &propapp.ClaimInfo{
		UserID: v.UserID, HolderName: v.HolderName, ClaimedAt: v.ClaimedAt,
		LastActivityAt: v.LastActivityAt, ExpiresAt: v.ExpiresAt(),
		CanRelease: queueapp.CanReleaseFor(ctx, v.UserID),
	}, nil
}

// --- Revisão para o proposition ---

type propReviews struct{ r *reviewapp.Reports }

func (p propReviews) Status(ctx context.Context, propositionID string) (propapp.ReviewStatus, error) {
	s, err := p.r.Status(ctx, propositionID)
	if err != nil {
		return propapp.ReviewStatus{}, err
	}
	out := propapp.ReviewStatus{
		CanApprove: s.Assessment.CanApprove(), ApprovalBreaksFourEyes: s.Assessment.BreaksFourEyes,
		CanSecondReview: s.CanSecondReview,
	}
	for _, b := range s.Assessment.Blockers {
		out.Blockers = append(out.Blockers, propapp.ReviewBlocker{Code: b.Code, Message: b.Message})
	}
	out.CanMinorFix, out.CanProposeErratum = s.CanMinorFix, s.CanProposeErratum
	for _, f := range s.MinorFixes {
		out.MinorFixes = append(out.MinorFixes, propapp.MinorFixInfo{
			ID: f.ID, VersionID: f.VersionID, Fields: f.Fields, Before: toPropSnapshot(f.Before), After: toPropSnapshot(f.After),
			CreatedBy: f.CreatedBy, CreatedAt: f.CreatedAt, CanCheck: f.CanCheck,
		})
	}
	if e := s.PendingErratum; e != nil {
		info := toErratumInfo(*e)
		out.PendingErratum = &info
	}
	for _, e := range s.Errata {
		out.Errata = append(out.Errata, toErratumInfo(e))
	}
	if a := s.Current; a != nil {
		out.Current = &propapp.ApprovalInfo{
			VersionID: a.VersionID, ApprovedBy: a.ApprovedBy, ApprovedAt: a.ApprovedAt, FourEyes: a.FourEyes,
			SecondReviewBy: a.SecondReviewBy, SecondReviewAt: a.SecondReviewAt,
			Badge: a.Badge(), PendingSecondReview: a.PendingSecondReview(),
		}
	}
	return out, nil
}

func toPropSnapshot(s reviewdomain.Snapshot) propapp.ContentSnapshot {
	return propapp.ContentSnapshot{Summary: s.Summary, ShortSummary: s.ShortSummary, TopicIDs: s.TopicIDs}
}

func toErratumInfo(e reviewapp.ErratumView) propapp.ErratumInfo {
	return propapp.ErratumInfo{
		ID: e.ID, VersionID: e.VersionID, Proposed: toPropSnapshot(e.Proposed), Said: e.Said, Correct: e.Correct,
		BaseRevision: e.BaseRevision, State: string(e.State), CreatedBy: e.CreatedBy, CreatedAt: e.CreatedAt,
		DecidedBy: e.DecidedBy, DecidedAt: e.DecidedAt, Reason: e.Reason,
		Stale: e.Stale, CanDecide: e.CanDecide, CanWithdraw: e.CanWithdraw,
	}
}

func (p propReviews) Badges(ctx context.Context, versionIDs []string) (map[string]int, error) {
	return p.r.Badges(ctx, versionIDs)
}

// ApprovedErrata leva à camada pública só a data e a nota das Erratas
// aprovadas: nem quem propôs, nem quem aprovou, nem o motivo. O filtro de
// estado se repete aqui de propósito (defesa em profundidade).
func (p propReviews) ApprovedErrata(ctx context.Context, versionIDs []string) ([]propdomain.PublicErratum, error) {
	es, err := p.r.ApprovedErrata(ctx, versionIDs)
	if err != nil {
		return nil, err
	}
	out := make([]propdomain.PublicErratum, 0, len(es))
	for _, e := range es {
		if e.State != reviewdomain.ErratumApproved || e.DecidedAt == nil {
			continue
		}
		out = append(out, propdomain.PublicErratum{VersionID: e.VersionID, ApprovedAt: *e.DecidedAt, Said: e.Said, Correct: e.Correct})
	}
	return out, nil
}

// --- Nomes da equipe ---

type propPeople struct{ d *teamapp.Directory }

func (p propPeople) Names(ctx context.Context, ids []string) (map[string]string, error) {
	ps, err := p.d.Lookup(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(ps))
	for id, person := range ps {
		out[id] = person.Name
	}
	return out, nil
}

type queuePeople struct{ d *teamapp.Directory }

func (q queuePeople) Lookup(ctx context.Context, ids []string) (map[string]queueapp.Person, error) {
	ps, err := q.d.Lookup(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]queueapp.Person, len(ps))
	for id, person := range ps {
		out[id] = queueapp.Person{Name: person.Name, Active: person.Active}
	}
	return out, nil
}

// --- Ciclo editorial para o review ---

type reviewEditorial struct{ e *propapp.Editorial }

// reviewErr traduz erros do proposition para os do review.
func reviewErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, propdomain.ErrPropositionNotFound):
		return errors.Join(reviewapp.ErrPropositionNotFound, err)
	case errors.Is(err, propdomain.ErrVersionNotFound):
		return errors.Join(reviewapp.ErrVersionNotFound, err)
	case errors.Is(err, propdomain.ErrVersionNotPending):
		return errors.Join(reviewapp.ErrVersionNotPending, err)
	case errors.Is(err, propdomain.ErrInvalidTransition):
		return errors.Join(reviewdomain.ErrNotInReview, err)
	}
	return err
}

func toSubject(s propapp.ApprovalSubject) reviewapp.Subject {
	out := reviewapp.Subject{
		PropositionID: s.PropositionID, Name: s.Name, InReview: s.ReviewState == propdomain.ReviewInReview,
		VersionID: s.Version.ID, VersionSeq: s.Version.Seq, Revision: s.Revision, Authors: s.Authors,
	}
	for _, m := range s.Missing {
		out.Missing = append(out.Missing, reviewdomain.Blocker{Code: m.Code, Message: m.Message})
	}
	return out
}

func (r reviewEditorial) Pending(ctx context.Context, propositionID string) (*reviewapp.Subject, error) {
	s, err := r.e.PendingSubject(ctx, propositionID)
	if err != nil || s == nil {
		return nil, reviewErr(err)
	}
	out := toSubject(*s)
	return &out, nil
}

func (r reviewEditorial) LockPending(ctx context.Context, propositionID, versionID string) (reviewapp.Subject, error) {
	s, err := r.e.LockForApproval(ctx, propositionID, versionID)
	if err != nil {
		return reviewapp.Subject{}, reviewErr(err)
	}
	return toSubject(s), nil
}

func (r reviewEditorial) Promote(ctx context.Context, propositionID, versionID string) error {
	return reviewErr(r.e.Promote(ctx, propositionID, versionID))
}

func (r reviewEditorial) Version(ctx context.Context, propositionID, versionID string) (reviewapp.VersionInfo, error) {
	v, authors, err := r.e.VersionAuthors(ctx, propositionID, versionID)
	if err != nil {
		return reviewapp.VersionInfo{}, reviewErr(err)
	}
	return reviewapp.VersionInfo{
		PropositionID: v.PropositionID, VersionID: v.ID, Seq: v.Seq,
		Current: v.State == propdomain.VersionCurrent, Authors: authors,
	}, nil
}

func (r reviewEditorial) CurrentVersionID(ctx context.Context, propositionID string) (string, error) {
	bs, err := r.e.Describe(ctx, []string{propositionID})
	if err != nil {
		return "", reviewErr(err)
	}
	if len(bs) == 0 {
		return "", reviewapp.ErrPropositionNotFound
	}
	if bs[0].Current == nil {
		return "", nil
	}
	return bs[0].Current.ID, nil
}

type reviewClaims struct{ g *queueapp.Gate }

func (c reviewClaims) Release(ctx context.Context, propositionID string) error {
	return c.g.Release(ctx, propositionID)
}

func (c reviewClaims) TouchForEdit(ctx context.Context, propositionID, userID string) error {
	err := c.g.TouchForEdit(ctx, propositionID, userID)
	var taken *queuedomain.ClaimTakenError
	if errors.As(err, &taken) {
		return &reviewapp.ClaimTakenError{HolderName: taken.HolderName}
	}
	if errors.Is(err, queueapp.ErrPropositionNotFound) {
		return errors.Join(reviewapp.ErrPropositionNotFound, err)
	}
	return err
}

// --- Conteúdo vigente para a Correção menor e a Errata ---

type reviewCorrections struct{ e *propapp.Editorial }

// invalidContent traduz as validações do conteúdo (proposition) nos
// códigos que a equipe já conhece da gravação do conteúdo.
var invalidContent = []struct {
	err           error
	code, message string
}{
	{propdomain.ErrSummaryTooLong, "summary_too_long", "O Resumo pode ter até 5.000 caracteres."},
	{propdomain.ErrShortSummaryTooLong, "short_summary_too_long", "O Resumo curto pode ter até 280 caracteres."},
	{propdomain.ErrTooManyTopics, "too_many_topics", "Escolha no máximo 3 Temas."},
	{propdomain.ErrDuplicateTopic, "duplicate_topic", "Tema repetido."},
	{propdomain.ErrTopicNotFound, "topic_not_found", "Tema não encontrado."},
	{propdomain.ErrTopicInactive, "topic_inactive", "Este Tema foi desativado e não pode ser escolhido."},
	{propdomain.ErrPublishedContentIncomplete, "published_content_incomplete", "O conteúdo publicado precisa de Resumo, Resumo curto e de 1 a 3 Temas."},
}

func correctionErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, propdomain.ErrPropositionNotFound):
		return errors.Join(reviewapp.ErrPropositionNotFound, err)
	case errors.Is(err, propdomain.ErrVersionNotFound):
		return errors.Join(reviewapp.ErrVersionNotFound, err)
	case errors.Is(err, propdomain.ErrVersionNotCurrent):
		return errors.Join(reviewdomain.ErrVersionNotCurrent, err)
	case errors.Is(err, propdomain.ErrStaleContent):
		return errors.Join(reviewdomain.ErrStaleContent, err)
	}
	for _, m := range invalidContent {
		if errors.Is(err, m.err) {
			return &reviewapp.InvalidContentError{Code: m.code, Message: m.message}
		}
	}
	return err
}

func toTarget(s propapp.CurrentSubject) reviewdomain.Target {
	return reviewdomain.Target{
		PropositionID: s.PropositionID, Name: s.Name, Ready: s.Ready, VersionID: s.Version.ID, Seq: s.Version.Seq,
		Content:  reviewdomain.Snapshot{Summary: s.Content.Summary, ShortSummary: s.Content.ShortSummary, TopicIDs: s.Content.TopicIDs},
		Revision: s.Content.Revision,
	}
}

func toEdit(s reviewdomain.Snapshot) propdomain.ContentEdit {
	return propdomain.ContentEdit{Summary: s.Summary, ShortSummary: s.ShortSummary, TopicIDs: s.TopicIDs}
}

func (r reviewCorrections) Current(ctx context.Context, propositionID string) (*reviewdomain.Target, error) {
	s, err := r.e.Current(ctx, propositionID)
	if err != nil || s == nil {
		return nil, correctionErr(err)
	}
	t := toTarget(*s)
	return &t, nil
}

func (r reviewCorrections) LockCurrent(ctx context.Context, propositionID, versionID string) (reviewdomain.Target, error) {
	s, err := r.e.LockCurrent(ctx, propositionID, versionID)
	if err != nil {
		return reviewdomain.Target{}, correctionErr(err)
	}
	return toTarget(s), nil
}

func (r reviewCorrections) CheckContent(ctx context.Context, proposed reviewdomain.Snapshot, previous []string) (reviewdomain.Snapshot, error) {
	n, err := r.e.CheckCurrentEdit(ctx, toEdit(proposed), previous)
	if err != nil {
		return reviewdomain.Snapshot{}, correctionErr(err)
	}
	return reviewdomain.Snapshot{Summary: n.Summary, ShortSummary: n.ShortSummary, TopicIDs: n.TopicIDs}, nil
}

func (r reviewCorrections) Rewrite(ctx context.Context, propositionID, versionID string, proposed reviewdomain.Snapshot, authorID string, revision int) (reviewapp.Rewrite, error) {
	c, fields, err := r.e.RewriteCurrent(ctx, propositionID, versionID, toEdit(proposed), authorID, revision)
	if err != nil {
		return reviewapp.Rewrite{}, correctionErr(err)
	}
	return reviewapp.Rewrite{
		Fields:   fields,
		Content:  reviewdomain.Snapshot{Summary: c.Summary, ShortSummary: c.ShortSummary, TopicIDs: c.TopicIDs},
		Revision: c.Revision,
	}, nil
}

// --- Proposições e Revisão para a Fila ---

type queueProps struct{ e *propapp.Editorial }

func toQueueVersion(v *propdomain.Version) *queueapp.Version {
	if v == nil {
		return nil
	}
	return &queueapp.Version{ID: v.ID, Seq: v.Seq, Label: v.Label, CreatedAt: v.CreatedAt}
}

func toQueueProps(bs []propapp.PropositionBrief) []queueapp.Proposition {
	out := make([]queueapp.Proposition, 0, len(bs))
	for _, b := range bs {
		p := queueapp.Proposition{
			ID: b.ID, Name: b.Name, Slug: b.Slug, Sphere: queuedomain.Sphere(b.Sphere),
			Ready: b.ReviewState == propdomain.ReviewReady, InReview: b.ReviewState == propdomain.ReviewInReview,
			Current: toQueueVersion(b.Current), Pending: toQueueVersion(b.Pending), Missing: []queueapp.Missing{},
		}
		for _, m := range b.PendingMissing {
			p.Missing = append(p.Missing, queueapp.Missing{Code: m.Code, Message: m.Message})
		}
		out = append(out, p)
	}
	return out
}

func (q queueProps) Exists(ctx context.Context, id string) (bool, error) { return q.e.Exists(ctx, id) }

func (q queueProps) InReview(ctx context.Context) ([]queueapp.Proposition, error) {
	bs, err := q.e.InReview(ctx)
	if err != nil {
		return nil, err
	}
	return toQueueProps(bs), nil
}

func (q queueProps) Describe(ctx context.Context, ids []string) ([]queueapp.Proposition, error) {
	bs, err := q.e.Describe(ctx, ids)
	if err != nil {
		return nil, err
	}
	return toQueueProps(bs), nil
}

type queueReviewWork struct{ r *reviewapp.Reports }

func (q queueReviewWork) PendingSecondReviews(ctx context.Context) ([]queueapp.PendingSecondReview, error) {
	as, err := q.r.PendingSecondReviews(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]queueapp.PendingSecondReview, 0, len(as))
	for _, a := range as {
		out = append(out, queueapp.PendingSecondReview{PropositionID: a.PropositionID, VersionID: a.VersionID, ApprovedAt: a.ApprovedAt})
	}
	return out, nil
}

func (q queueReviewWork) PendingCorrections(ctx context.Context) ([]queueapp.PendingCorrection, error) {
	fixes, err := q.r.UncheckedMinorFixes(ctx)
	if err != nil {
		return nil, err
	}
	errata, err := q.r.PendingErrata(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]queueapp.PendingCorrection, 0, len(fixes)+len(errata))
	for _, f := range fixes {
		out = append(out, queueapp.PendingCorrection{PropositionID: f.PropositionID, VersionID: f.VersionID, Reason: queuedomain.ReasonMinorFixCheck, Since: f.CreatedAt})
	}
	for _, e := range errata {
		out = append(out, queueapp.PendingCorrection{PropositionID: e.PropositionID, VersionID: e.VersionID, Reason: queuedomain.ReasonErratum, Since: e.CreatedAt})
	}
	return out, nil
}

// --- Auditoria ---

type reviewAudit struct{ record recordAudit }

func (a reviewAudit) Record(ctx context.Context, e reviewapp.AuditEvent) error {
	return a.record(ctx, auditapp.RecordEventInput{
		ActorID: e.ActorID, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID, Metadata: e.Metadata,
	})
}

type queueAudit struct{ record recordAudit }

func (a queueAudit) Record(ctx context.Context, e queueapp.AuditEvent) error {
	return a.record(ctx, auditapp.RecordEventInput{
		ActorID: e.ActorID, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID, Metadata: e.Metadata,
	})
}
