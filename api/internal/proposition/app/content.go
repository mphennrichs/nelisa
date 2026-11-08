package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// EditorialRepos são os repositórios do ciclo editorial.
type EditorialRepos struct {
	Props        PropositionRepo
	Versions     VersionRepo
	Contents     ContentRepo
	Topics       TopicRepo
	Structurings StructuringRepo
}

// --- Versão nova ---

type CreateVersionInput struct {
	PropositionID   string
	Label           string
	OfficialTextURL string
}

// CreateVersionResult é a Versão criada e o conteúdo dela (copiado da
// vigente, a confirmar, ou vazio).
type CreateVersionResult struct {
	Version domain.Version
	Content domain.Content
}

// CreateVersion é o Editor registrando uma edição nova do Documento
// integral (substitutivo, emenda, nova redação). Só sem Versão em análise.
// A Proposição pronta volta para "em revisão", o conteúdo da vigente vem
// marcado a confirmar e quem cria assume a Posse (ou leva 409 se ela é de
// outra pessoa): registrar a Versão é começar a trabalhar nela.
type CreateVersion struct {
	repos  EditorialRepos
	claims Claims
	sched  StructuringScheduler
	tx     TxRunner
	audit  AuditRecorder
	clock  clock.Clock
}

func NewCreateVersion(repos EditorialRepos, claims Claims, sched StructuringScheduler, tx TxRunner, audit AuditRecorder, clk clock.Clock) *CreateVersion {
	return &CreateVersion{repos: repos, claims: claims, sched: sched, tx: tx, audit: audit, clock: clk}
}

func (uc *CreateVersion) Execute(ctx context.Context, in CreateVersionInput) (CreateVersionResult, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return CreateVersionResult{}, err
	}
	var out CreateVersionResult
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := uc.repos.Props.FindByIDForUpdate(ctx, in.PropositionID)
		if err != nil {
			return err
		}
		versions, err := uc.repos.Versions.ListByProposition(ctx, p.ID)
		if err != nil {
			return err
		}
		current, pending := domain.FindVersions(versions)
		if pending != nil {
			return domain.ErrPendingVersionExists
		}
		id, err := newID()
		if err != nil {
			return err
		}
		now := uc.clock.Now()
		v, err := domain.NewVersion(id, p.ID, len(versions)+1, in.Label, in.OfficialTextURL, actor.UserID, now)
		if err != nil {
			return err
		}
		if err := uc.claims.TouchForEdit(ctx, p.ID, actor.UserID); err != nil {
			return err
		}
		content := domain.NewEmptyContent(v.ID, now)
		if current != nil {
			prev, err := uc.repos.Contents.FindByVersion(ctx, current.ID)
			if err != nil {
				return err
			}
			content = prev.CopyForNewVersion(v.ID, now)
		}
		if err := uc.repos.Versions.Create(ctx, v); err != nil {
			return err
		}
		if err := uc.repos.Contents.Create(ctx, content); err != nil {
			return err
		}
		if err := startStructuring(ctx, uc.repos.Structurings, uc.sched, v, now); err != nil {
			return err
		}
		from := p.ReviewState
		p.ReturnToReview(now)
		if p.ReviewState != from {
			if err := uc.repos.Props.Update(ctx, p); err != nil {
				return err
			}
		}
		out = CreateVersionResult{Version: v, Content: content}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionVersionCreate, TargetType: targetVersion, TargetID: v.ID,
			Metadata: map[string]any{
				"proposition_id": p.ID, "seq": v.Seq, "label": v.Label, "official_text_url": v.OfficialTextURL,
				"copied_content": current != nil, "review_state_from": string(from), "review_state_to": string(p.ReviewState),
			},
		})
	})
	if err != nil {
		return CreateVersionResult{}, err
	}
	return out, nil
}

// --- gravar conteúdo ---

type UpdateContentInput struct {
	PropositionID string
	VersionID     string
	Summary       string
	ShortSummary  string
	TopicIDs      []string
	// Revision é a revisão lida pelo Editor (trava otimista).
	Revision int
}

// UpdateContent grava Resumo, Resumo curto e Temas da Versão em análise.
// Exige a Posse (assume se livre; 409 se de outra pessoa) e a revisão
// lida (409 se alguém gravou depois). Quem grava entra na Autoria. Salvar
// não confirma o conteúdo trazido.
type UpdateContent struct {
	repos  EditorialRepos
	claims Claims
	tx     TxRunner
	audit  AuditRecorder
	clock  clock.Clock
}

func NewUpdateContent(repos EditorialRepos, claims Claims, tx TxRunner, audit AuditRecorder, clk clock.Clock) *UpdateContent {
	return &UpdateContent{repos: repos, claims: claims, tx: tx, audit: audit, clock: clk}
}

func (uc *UpdateContent) Execute(ctx context.Context, in UpdateContentInput) (domain.Content, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return domain.Content{}, err
	}
	edit, err := domain.ContentEdit{Summary: in.Summary, ShortSummary: in.ShortSummary, TopicIDs: in.TopicIDs}.Normalize()
	if err != nil {
		return domain.Content{}, err
	}
	var out domain.Content
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, v, err := editableVersion(ctx, uc.repos, in.PropositionID, in.VersionID)
		if err != nil {
			return err
		}
		if err := uc.claims.TouchForEdit(ctx, p.ID, actor.UserID); err != nil {
			return err
		}
		c, err := uc.repos.Contents.FindByVersion(ctx, v.ID)
		if err != nil {
			return err
		}
		if c.Revision != in.Revision {
			return domain.ErrStaleContent
		}
		topics, err := uc.repos.Topics.List(ctx)
		if err != nil {
			return err
		}
		if err := domain.CheckTopicChoice(edit.TopicIDs, c.TopicIDs, domain.TopicsByID(topics)); err != nil {
			return err
		}
		prev := c.Revision
		changed := c.Apply(edit, actor.UserID, uc.clock.Now())
		out = c
		if len(changed) == 0 {
			return nil // nada mudou: só a atividade da Posse foi renovada
		}
		if err := uc.repos.Contents.Update(ctx, c, prev); err != nil {
			return err
		}
		// Metadado só: Versão e campos alterados, nunca o texto.
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionContentUpdate, TargetType: targetVersion, TargetID: v.ID,
			Metadata: map[string]any{"proposition_id": p.ID, "seq": v.Seq, "fields": changed, "revision": c.Revision},
		})
	})
	if err != nil {
		return domain.Content{}, err
	}
	return out, nil
}

// --- confirmar conteúdo trazido ---

type ConfirmContentInput struct {
	PropositionID string
	VersionID     string
	// Revision é a revisão que o Editor viu: confirma-se o que se leu.
	Revision int
}

// ConfirmContent é a ação explícita "confirmar conteúdo trazido" da
// Versão anterior. Exige a Posse como gravar; quem confirma entra na
// Autoria.
type ConfirmContent struct {
	repos  EditorialRepos
	claims Claims
	tx     TxRunner
	audit  AuditRecorder
	clock  clock.Clock
}

func NewConfirmContent(repos EditorialRepos, claims Claims, tx TxRunner, audit AuditRecorder, clk clock.Clock) *ConfirmContent {
	return &ConfirmContent{repos: repos, claims: claims, tx: tx, audit: audit, clock: clk}
}

func (uc *ConfirmContent) Execute(ctx context.Context, in ConfirmContentInput) (domain.Content, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return domain.Content{}, err
	}
	var out domain.Content
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, v, err := editableVersion(ctx, uc.repos, in.PropositionID, in.VersionID)
		if err != nil {
			return err
		}
		if err := uc.claims.TouchForEdit(ctx, p.ID, actor.UserID); err != nil {
			return err
		}
		c, err := uc.repos.Contents.FindByVersion(ctx, v.ID)
		if err != nil {
			return err
		}
		if c.Revision != in.Revision {
			return domain.ErrStaleContent
		}
		prev := c.Revision
		if err := c.Confirm(actor.UserID, uc.clock.Now()); err != nil {
			return err
		}
		if err := uc.repos.Contents.Update(ctx, c, prev); err != nil {
			return err
		}
		out = c
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionContentConfirm, TargetType: targetVersion, TargetID: v.ID,
			Metadata: map[string]any{"proposition_id": p.ID, "seq": v.Seq, "revision": c.Revision},
		})
	})
	if err != nil {
		return domain.Content{}, err
	}
	return out, nil
}

// editableVersion trava a Proposição e confere que a Versão é dela e está
// em análise: só essa tem conteúdo editável.
func editableVersion(ctx context.Context, repos EditorialRepos, propositionID, versionID string) (domain.Proposition, domain.Version, error) {
	p, err := repos.Props.FindByIDForUpdate(ctx, propositionID)
	if err != nil {
		return domain.Proposition{}, domain.Version{}, err
	}
	v, err := repos.Versions.FindByID(ctx, versionID)
	if err != nil {
		return domain.Proposition{}, domain.Version{}, err
	}
	if v.PropositionID != p.ID {
		return domain.Proposition{}, domain.Version{}, domain.ErrVersionNotFound
	}
	if v.State != domain.VersionPending {
		return domain.Proposition{}, domain.Version{}, domain.ErrVersionNotEditable
	}
	return p, v, nil
}
