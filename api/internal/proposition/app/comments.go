package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// Comentários: escritos na Versão em análise, depois de confirmada a
// Estruturação, com a Posse e trava otimista, como o Resumo. Quem escreve
// entra na Autoria do Comentário e do conteúdo da Versão: os quatro olhos
// valem para tudo o que o aprovador vai assinar. Nunca escritos por IA
// (ADR 0003).

// Ações auditadas dos Comentários.
const (
	ActionCommentCreate  = "comment.create"
	ActionCommentUpdate  = "comment.update"
	ActionCommentDelete  = "comment.delete"
	ActionCommentConfirm = "comment.confirm"

	targetComment = "comment"
)

// commentTx abre a transação comum: Versão em análise, Posse, e no fim a
// Autoria do conteúdo e a Auditoria.
func (d StructuringEdit) commentTx(ctx context.Context, propositionID, versionID, action string,
	fn func(ctx context.Context, actorID string, v domain.Version) (domain.Comment, error),
) (domain.Comment, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return domain.Comment{}, err
	}
	var out domain.Comment
	err = d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		p, v, err := editableVersion(ctx, d.Repos, propositionID, versionID)
		if err != nil {
			return err
		}
		if err := d.Claims.TouchForEdit(ctx, p.ID, actor.UserID); err != nil {
			return err
		}
		c, err := fn(ctx, actor.UserID, v)
		if err != nil {
			return err
		}
		content, err := d.Repos.Contents.FindByVersion(ctx, v.ID)
		if err != nil {
			return err
		}
		prev := content.Revision
		content.RecordAuthor(actor.UserID, d.Clock.Now())
		if err := d.Repos.Contents.Update(ctx, content, prev); err != nil {
			return err
		}
		out = c
		// Metadado só: nunca o texto.
		return d.Audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: action, TargetType: targetComment, TargetID: c.ID,
			Metadata: map[string]any{
				"proposition_id": p.ID, "version_id": v.ID, "seq": v.Seq,
				"provision_address": c.ProvisionAddress, "revision": c.Revision,
			},
		})
	})
	if err != nil {
		return domain.Comment{}, err
	}
	return out, nil
}

// lockComment traz o Comentário da Versão, vivo e com a revisão lida.
func (d StructuringEdit) lockComment(ctx context.Context, v domain.Version, commentID string, revision int) (domain.Comment, error) {
	c, err := d.Repos.Comments.FindForUpdate(ctx, commentID)
	if err != nil {
		return domain.Comment{}, err
	}
	if c.VersionID != v.ID || c.DeletedAt != nil {
		return domain.Comment{}, domain.ErrCommentNotFound
	}
	if c.Revision != revision {
		return domain.Comment{}, domain.ErrStaleContent
	}
	return c, nil
}

type CreateCommentInput struct {
	PropositionID    string
	VersionID        string
	ProvisionAddress string
	Body             string
}

type CreateComment struct{ d StructuringEdit }

func NewCreateComment(d StructuringEdit) *CreateComment { return &CreateComment{d: d} }

func (uc *CreateComment) Execute(ctx context.Context, in CreateCommentInput) (domain.Comment, error) {
	body, err := domain.NormalizeCommentBody(in.Body)
	if err != nil {
		return domain.Comment{}, err
	}
	return uc.d.commentTx(ctx, in.PropositionID, in.VersionID, ActionCommentCreate,
		func(ctx context.Context, actorID string, v domain.Version) (domain.Comment, error) {
			s, err := uc.d.Repos.Structurings.FindByVersionForUpdate(ctx, v.ID)
			if err != nil {
				return domain.Comment{}, err
			}
			ps, err := uc.d.Repos.Structurings.ListProvisions(ctx, v.ID)
			if err != nil {
				return domain.Comment{}, err
			}
			if err := domain.CheckCommentable(s, ps, in.ProvisionAddress); err != nil {
				return domain.Comment{}, err
			}
			id, err := newID()
			if err != nil {
				return domain.Comment{}, err
			}
			c := domain.NewComment(id, v.ID, in.ProvisionAddress, body, actorID, uc.d.Clock.Now())
			return c, uc.d.Repos.Comments.Create(ctx, c)
		})
}

type UpdateCommentInput struct {
	PropositionID string
	VersionID     string
	CommentID     string
	Body          string
	Revision      int
}

type UpdateComment struct{ d StructuringEdit }

func NewUpdateComment(d StructuringEdit) *UpdateComment { return &UpdateComment{d: d} }

func (uc *UpdateComment) Execute(ctx context.Context, in UpdateCommentInput) (domain.Comment, error) {
	body, err := domain.NormalizeCommentBody(in.Body)
	if err != nil {
		return domain.Comment{}, err
	}
	return uc.d.commentTx(ctx, in.PropositionID, in.VersionID, ActionCommentUpdate,
		func(ctx context.Context, actorID string, v domain.Version) (domain.Comment, error) {
			c, err := uc.d.lockComment(ctx, v, in.CommentID, in.Revision)
			if err != nil {
				return domain.Comment{}, err
			}
			prev := c.Revision
			if !c.Edit(body, actorID, uc.d.Clock.Now()) {
				return c, nil
			}
			return c, uc.d.Repos.Comments.Update(ctx, c, prev)
		})
}

type CommentActionInput struct {
	PropositionID string
	VersionID     string
	CommentID     string
	Revision      int
}

// DeleteComment apaga (soft) o Comentário.
type DeleteComment struct{ d StructuringEdit }

func NewDeleteComment(d StructuringEdit) *DeleteComment { return &DeleteComment{d: d} }

func (uc *DeleteComment) Execute(ctx context.Context, in CommentActionInput) (domain.Comment, error) {
	return uc.d.commentTx(ctx, in.PropositionID, in.VersionID, ActionCommentDelete,
		func(ctx context.Context, actorID string, v domain.Version) (domain.Comment, error) {
			c, err := uc.d.lockComment(ctx, v, in.CommentID, in.Revision)
			if err != nil {
				return domain.Comment{}, err
			}
			prev := c.Revision
			c.Delete(actorID, uc.d.Clock.Now())
			return c, uc.d.Repos.Comments.Update(ctx, c, prev)
		})
}

// ConfirmComment confirma um Comentário trazido da Versão anterior.
type ConfirmComment struct{ d StructuringEdit }

func NewConfirmComment(d StructuringEdit) *ConfirmComment { return &ConfirmComment{d: d} }

func (uc *ConfirmComment) Execute(ctx context.Context, in CommentActionInput) (domain.Comment, error) {
	return uc.d.commentTx(ctx, in.PropositionID, in.VersionID, ActionCommentConfirm,
		func(ctx context.Context, actorID string, v domain.Version) (domain.Comment, error) {
			c, err := uc.d.lockComment(ctx, v, in.CommentID, in.Revision)
			if err != nil {
				return domain.Comment{}, err
			}
			prev := c.Revision
			if err := c.Confirm(actorID, uc.d.Clock.Now()); err != nil {
				return domain.Comment{}, err
			}
			return c, uc.d.Repos.Comments.Update(ctx, c, prev)
		})
}

// carryComments traz, na confirmação da Estruturação de uma Versão nova,
// os Comentários da vigente cujos Dispositivos não mudaram.
func (d StructuringEdit) carryComments(ctx context.Context, v domain.Version) (int, error) {
	vs, err := d.Repos.Versions.ListByProposition(ctx, v.PropositionID)
	if err != nil {
		return 0, err
	}
	current, _ := domain.FindVersions(vs)
	if current == nil || current.ID == v.ID {
		return 0, nil
	}
	cs, err := d.Repos.Comments.ListByVersions(ctx, []string{current.ID, v.ID})
	if err != nil {
		return 0, err
	}
	var from []domain.Comment
	already := map[string]bool{}
	for _, c := range cs {
		switch c.VersionID {
		case current.ID:
			from = append(from, c)
		case v.ID:
			if c.CopiedFrom != "" {
				already[c.CopiedFrom] = true
			}
		}
	}
	if len(from) == 0 {
		return 0, nil
	}
	fromPs, err := d.Repos.Structurings.ListProvisions(ctx, current.ID)
	if err != nil {
		return 0, err
	}
	toPs, err := d.Repos.Structurings.ListProvisions(ctx, v.ID)
	if err != nil {
		return 0, err
	}
	carry := domain.CarryComments(from, fromPs, toPs, already)
	now := d.Clock.Now()
	for _, c := range carry {
		id, err := newID()
		if err != nil {
			return 0, err
		}
		if err := d.Repos.Comments.Create(ctx, c.CopyForNewVersion(id, v.ID, now)); err != nil {
			return 0, err
		}
	}
	return len(carry), nil
}
