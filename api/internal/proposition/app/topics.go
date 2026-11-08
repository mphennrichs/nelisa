package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// requireAdmin: a lista de Temas é mantida só pelo Admin.
func requireAdmin(ctx context.Context) (auth.Actor, error) {
	a, ok := auth.ActorFrom(ctx)
	if !ok {
		return auth.Actor{}, ErrUnauthenticated
	}
	if !a.Role.AtLeast(auth.RoleAdmin) {
		return a, ErrForbidden
	}
	return a, nil
}

// ListTopics lista todos os Temas (ativos e desativados) para a equipe.
type ListTopics struct{ topics TopicRepo }

func NewListTopics(topics TopicRepo) *ListTopics { return &ListTopics{topics: topics} }

func (uc *ListTopics) Execute(ctx context.Context) ([]domain.Topic, error) {
	if _, err := requireEditor(ctx); err != nil {
		return nil, err
	}
	return uc.topics.List(ctx)
}

type CreateTopicInput struct{ Name string }

// CreateTopic é o Admin acrescentando um Tema à lista fechada.
type CreateTopic struct {
	topics TopicRepo
	tx     TxRunner
	audit  AuditRecorder
	clock  clock.Clock
}

func NewCreateTopic(topics TopicRepo, tx TxRunner, audit AuditRecorder, clk clock.Clock) *CreateTopic {
	return &CreateTopic{topics: topics, tx: tx, audit: audit, clock: clk}
}

func (uc *CreateTopic) Execute(ctx context.Context, in CreateTopicInput) (domain.Topic, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return domain.Topic{}, err
	}
	id, err := newID()
	if err != nil {
		return domain.Topic{}, err
	}
	t, err := domain.NewTopic(id, in.Name, uc.clock.Now())
	if err != nil {
		return domain.Topic{}, err
	}
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.topics.Create(ctx, t); err != nil {
			return err
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionTopicCreate, TargetType: targetTopic, TargetID: t.ID,
			Metadata: map[string]any{"name": t.Name},
		})
	})
	if err != nil {
		return domain.Topic{}, err
	}
	return t, nil
}

type UpdateTopicInput struct {
	TopicID string
	// Name e Active nil não mudam.
	Name   *string
	Active *bool
}

// UpdateTopic renomeia, desativa ou reativa um Tema (Admin). Desativado
// some da escolha, mas continua nas Versões que já o usam.
type UpdateTopic struct {
	topics TopicRepo
	tx     TxRunner
	audit  AuditRecorder
}

func NewUpdateTopic(topics TopicRepo, tx TxRunner, audit AuditRecorder) *UpdateTopic {
	return &UpdateTopic{topics: topics, tx: tx, audit: audit}
}

func (uc *UpdateTopic) Execute(ctx context.Context, in UpdateTopicInput) (domain.Topic, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return domain.Topic{}, err
	}
	var name string
	if in.Name != nil {
		if name, err = domain.NormalizeTopicName(*in.Name); err != nil {
			return domain.Topic{}, err
		}
	}
	var out domain.Topic
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		t, err := uc.topics.FindByID(ctx, in.TopicID)
		if err != nil {
			return err
		}
		var events []AuditEvent
		if in.Name != nil && name != t.Name {
			events = append(events, AuditEvent{Action: ActionTopicRename, Metadata: map[string]any{"from": t.Name, "to": name}})
			t.Name = name
		}
		if in.Active != nil && *in.Active != t.Active {
			action := ActionTopicDeactivate
			if *in.Active {
				action = ActionTopicReactivate
			}
			events = append(events, AuditEvent{Action: action, Metadata: map[string]any{"name": name}})
			t.Active = *in.Active
		}
		out = t
		if len(events) == 0 {
			return nil
		}
		if err := uc.topics.Update(ctx, t); err != nil {
			return err
		}
		for _, e := range events {
			e.ActorID, e.TargetType, e.TargetID = actor.UserID, targetTopic, t.ID
			if e.Action != ActionTopicRename {
				e.Metadata["name"] = t.Name
			}
			if err := uc.audit.Record(ctx, e); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.Topic{}, err
	}
	return out, nil
}
