package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// StructuringEdit agrupa o que os casos de uso da Estruturação usam.
type StructuringEdit struct {
	Repos  EditorialRepos
	Claims Claims
	Sched  StructuringScheduler
	Tx     TxRunner
	Audit  AuditRecorder
	Clock  clock.Clock
}

// StructuringView é a Estruturação com os Dispositivos, como a equipe a vê.
type StructuringView struct {
	domain.Structuring
	Provisions []domain.Provision
}

// editStructuring abre a transação comum: Versão em análise, Posse
// (assume se livre, 409 se de outra pessoa), Estruturação travada e com a
// revisão que o Editor leu. fn muda s; depois disso a Estruturação é
// gravada, quem mexeu (authored) entra na Autoria do conteúdo (quatro
// olhos) e a Auditoria registra.
func (d StructuringEdit) edit(ctx context.Context, propositionID, versionID string, revision *int, action string, authored bool,
	fn func(ctx context.Context, actorID string, v domain.Version, s *domain.Structuring) (map[string]any, error),
) (StructuringView, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return StructuringView{}, err
	}
	var out StructuringView
	err = d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		p, v, err := editableVersion(ctx, d.Repos, propositionID, versionID)
		if err != nil {
			return err
		}
		if err := d.Claims.TouchForEdit(ctx, p.ID, actor.UserID); err != nil {
			return err
		}
		s, err := d.Repos.Structurings.FindByVersionForUpdate(ctx, v.ID)
		if err != nil {
			return err
		}
		if revision != nil && s.Revision != *revision {
			return domain.ErrStaleContent
		}
		prev := s.Revision
		meta, err := fn(ctx, actor.UserID, v, &s)
		if err != nil {
			return err
		}
		if err := d.Repos.Structurings.Update(ctx, s, prev); err != nil {
			return err
		}
		if authored {
			c, err := d.Repos.Contents.FindByVersion(ctx, v.ID)
			if err != nil {
				return err
			}
			prevContent := c.Revision
			c.RecordAuthor(actor.UserID, d.Clock.Now())
			if err := d.Repos.Contents.Update(ctx, c, prevContent); err != nil {
				return err
			}
		}
		ps, err := d.Repos.Structurings.ListProvisions(ctx, v.ID)
		if err != nil {
			return err
		}
		out = StructuringView{Structuring: s, Provisions: ps}
		meta["proposition_id"], meta["seq"], meta["revision"] = p.ID, v.Seq, s.Revision
		return d.Audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: action, TargetType: targetVersion, TargetID: v.ID, Metadata: meta,
		})
	})
	if err != nil {
		return StructuringView{}, err
	}
	return out, nil
}

type ReprocessStructuringInput struct {
	PropositionID string
	VersionID     string
	// SourceText é o texto corrigido ou colado pelo Editor.
	SourceText string
	Revision   int
}

// ReprocessStructuring grava o texto que o Editor corrigiu (ou colou, se a
// busca falhou) e refaz os Dispositivos. Também vale enquanto a busca
// automática não terminou: ela vê que não está mais em "fetching" e para.
type ReprocessStructuring struct{ d StructuringEdit }

func NewReprocessStructuring(d StructuringEdit) *ReprocessStructuring {
	return &ReprocessStructuring{d: d}
}

func (uc *ReprocessStructuring) Execute(ctx context.Context, in ReprocessStructuringInput) (StructuringView, error) {
	return uc.d.edit(ctx, in.PropositionID, in.VersionID, &in.Revision, ActionStructuringReprocess, true,
		func(ctx context.Context, actorID string, v domain.Version, s *domain.Structuring) (map[string]any, error) {
			r, err := s.Reprocess(in.SourceText, actorID, uc.d.Clock.Now())
			if err != nil {
				return nil, err
			}
			if err := uc.d.Repos.Structurings.ReplaceProvisions(ctx, v.ID, domain.ProvisionsFrom(v.ID, r)); err != nil {
				return nil, err
			}
			// Metadado só: nunca o texto.
			return map[string]any{"provisions": len(r.Provisions), "warnings": len(r.Warnings)}, nil
		})
}

type ConfirmStructuringInput struct {
	PropositionID string
	VersionID     string
	Revision      int
}

// ConfirmStructuring: o Editor conferiu os Dispositivos.
type ConfirmStructuring struct{ d StructuringEdit }

func NewConfirmStructuring(d StructuringEdit) *ConfirmStructuring { return &ConfirmStructuring{d: d} }

func (uc *ConfirmStructuring) Execute(ctx context.Context, in ConfirmStructuringInput) (StructuringView, error) {
	return uc.d.edit(ctx, in.PropositionID, in.VersionID, &in.Revision, ActionStructuringConfirm, true,
		func(_ context.Context, actorID string, _ domain.Version, s *domain.Structuring) (map[string]any, error) {
			return map[string]any{}, s.Confirm(actorID, uc.d.Clock.Now())
		})
}

type RetryStructuringInput struct {
	PropositionID string
	VersionID     string
}

// RetryStructuring volta a buscar o PDF oficial de uma Estruturação manual.
type RetryStructuring struct{ d StructuringEdit }

func NewRetryStructuring(d StructuringEdit) *RetryStructuring { return &RetryStructuring{d: d} }

func (uc *RetryStructuring) Execute(ctx context.Context, in RetryStructuringInput) (StructuringView, error) {
	return uc.d.edit(ctx, in.PropositionID, in.VersionID, nil, ActionStructuringRetry, false,
		func(ctx context.Context, actorID string, v domain.Version, s *domain.Structuring) (map[string]any, error) {
			failure := s.Failure
			if err := s.Retry(v, actorID, uc.d.Clock.Now()); err != nil {
				return nil, err
			}
			return map[string]any{"previous_failure": string(failure)}, uc.d.Sched.ScheduleStructuring(ctx, v.ID)
		})
}
