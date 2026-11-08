package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// Operações de sistema que a Correção menor e a Errata (módulo review)
// usam, ligadas no main. Como o resto de Editorial: não autorizam, não
// auditam e não abrem transação; o caso de uso do review faz as três
// coisas. O conteúdo continua sendo deste módulo: só ele grava.

// CurrentSubject é a Versão vigente e o conteúdo dela, como a Correção
// menor e a Errata os enxergam.
type CurrentSubject struct {
	PropositionID string
	Name          string
	Ready         bool
	Version       domain.Version
	Content       domain.Content
}

func (e *Editorial) currentSubject(ctx context.Context, p domain.Proposition, v domain.Version) (CurrentSubject, error) {
	c, err := e.repos.Contents.FindByVersion(ctx, v.ID)
	if err != nil {
		return CurrentSubject{}, err
	}
	return CurrentSubject{PropositionID: p.ID, Name: p.Name(), Ready: p.ReviewState == domain.ReviewReady, Version: v, Content: c}, nil
}

// Current devolve a Versão vigente da Proposição, ou nil se não há.
func (e *Editorial) Current(ctx context.Context, propositionID string) (*CurrentSubject, error) {
	p, err := e.repos.Props.FindByID(ctx, propositionID)
	if err != nil {
		return nil, err
	}
	vs, err := e.repos.Versions.ListByProposition(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	current, _ := domain.FindVersions(vs)
	if current == nil {
		return nil, nil
	}
	s, err := e.currentSubject(ctx, p, *current)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// LockCurrent trava a Proposição até o fim da transação do contexto (a
// mesma trava de quem grava conteúdo e de quem aprova) e devolve a Versão
// pedida, que tem de ser dela e a vigente.
func (e *Editorial) LockCurrent(ctx context.Context, propositionID, versionID string) (CurrentSubject, error) {
	p, err := e.repos.Props.FindByIDForUpdate(ctx, propositionID)
	if err != nil {
		return CurrentSubject{}, err
	}
	v, err := e.currentVersion(ctx, p.ID, versionID)
	if err != nil {
		return CurrentSubject{}, err
	}
	return e.currentSubject(ctx, p, v)
}

func (e *Editorial) currentVersion(ctx context.Context, propositionID, versionID string) (domain.Version, error) {
	v, err := e.repos.Versions.FindByID(ctx, versionID)
	if err != nil {
		return domain.Version{}, err
	}
	if v.PropositionID != propositionID {
		return domain.Version{}, domain.ErrVersionNotFound
	}
	if v.State != domain.VersionCurrent {
		return domain.Version{}, domain.ErrVersionNotCurrent
	}
	return v, nil
}

// CheckCurrentEdit aplica a uma edição do conteúdo vigente as mesmas
// validações da gravação do conteúdo, mais as do conteúdo publicado
// (CheckPublishable), e devolve a edição normalizada. previous são os
// Temas que a Versão já usa (um Tema desativado depois continua valendo
// para ela).
func (e *Editorial) CheckCurrentEdit(ctx context.Context, edit domain.ContentEdit, previous []string) (domain.ContentEdit, error) {
	n, err := edit.Normalize()
	if err != nil {
		return domain.ContentEdit{}, err
	}
	if err := n.CheckPublishable(); err != nil {
		return domain.ContentEdit{}, err
	}
	topics, err := e.repos.Topics.List(ctx)
	if err != nil {
		return domain.ContentEdit{}, err
	}
	if err := domain.CheckTopicChoice(n.TopicIDs, previous, domain.TopicsByID(topics)); err != nil {
		return domain.ContentEdit{}, err
	}
	return n, nil
}

// RewriteCurrent troca o conteúdo da Versão vigente pela edição (já
// conferida por CheckCurrentEdit), com authorID entrando na Autoria, se a
// revision guardada ainda for revision. Chame dentro da transação de
// LockCurrent. Devolve o conteúdo como ficou e os campos que mudaram; sem
// mudança, nada é gravado.
func (e *Editorial) RewriteCurrent(ctx context.Context, propositionID, versionID string, edit domain.ContentEdit, authorID string, revision int) (domain.Content, []string, error) {
	v, err := e.currentVersion(ctx, propositionID, versionID)
	if err != nil {
		return domain.Content{}, nil, err
	}
	c, err := e.repos.Contents.FindByVersion(ctx, v.ID)
	if err != nil {
		return domain.Content{}, nil, err
	}
	if c.Revision != revision {
		return domain.Content{}, nil, domain.ErrStaleContent
	}
	changed := c.Apply(edit, authorID, e.clock.Now())
	if len(changed) == 0 {
		return c, nil, nil
	}
	if err := e.repos.Contents.Update(ctx, c, revision); err != nil {
		return domain.Content{}, nil, err
	}
	return c, changed, nil
}
