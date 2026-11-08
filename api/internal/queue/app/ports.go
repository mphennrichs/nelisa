// Package app tem os casos de uso da Posse e da Fila de revisão, as
// operações de Posse que o módulo proposition (gravar conteúdo) e o review
// (aprovar solta a Posse) usam, e as portas de que precisam. A autorização
// acontece aqui, com o Ator vindo do contexto.
package app

import (
	"context"
	"errors"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
)

var (
	ErrUnauthenticated = errors.New("queue: não autenticado")
	ErrForbidden       = errors.New("queue: sem permissão")
	// ErrPropositionNotFound: a Proposição não existe (o módulo
	// proposition responde, ligado no main).
	ErrPropositionNotFound = errors.New("queue: proposição não encontrada")
)

// ClaimRepo persiste a Posse.
type ClaimRepo interface {
	// Lock serializa, até o fim da transação do contexto, quem mexe na
	// Posse da Proposição (exista a linha ou não).
	Lock(ctx context.Context, propositionID string) error
	// Find devolve a linha guardada, ou nil. Pode estar expirada.
	Find(ctx context.Context, propositionID string) (*domain.Claim, error)
	ListByPropositions(ctx context.Context, propositionIDs []string) ([]domain.Claim, error)
	Save(ctx context.Context, c domain.Claim) error
	Delete(ctx context.Context, propositionID string) error
}

// Person é alguém da equipe, como a Fila precisa: nome e se a conta está
// ativa.
type Person struct {
	Name   string
	Active bool
}

// People é o cadastro da equipe (módulo team, ligado no main). Id
// desconhecido fica de fora (e vale como conta inativa).
type People interface {
	Lookup(ctx context.Context, ids []string) (map[string]Person, error)
}

// Version é a Versão de um item da Fila.
type Version struct {
	ID        string
	Seq       int
	Label     string
	CreatedAt time.Time
}

// Missing é o que falta no conteúdo para aprovar.
type Missing struct {
	Code    string
	Message string
}

// Proposition é o que a Fila precisa de uma Proposição.
type Proposition struct {
	ID       string
	Name     string
	Slug     string
	Sphere   domain.Sphere
	Ready    bool
	InReview bool
	Current  *Version
	Pending  *Version
	// Missing é o que falta para aprovar a Versão em análise.
	Missing []Missing
}

// Propositions é o módulo proposition, ligado no main.
type Propositions interface {
	Exists(ctx context.Context, id string) (bool, error)
	// InReview lista as Proposições "em revisão" com Versão em análise.
	InReview(ctx context.Context) ([]Proposition, error)
	Describe(ctx context.Context, ids []string) ([]Proposition, error)
}

// PendingSecondReview é uma aprovação sem quatro olhos ainda sem a segunda
// revisão.
type PendingSecondReview struct {
	PropositionID string
	VersionID     string
	ApprovedAt    time.Time
}

// SecondReviews é o módulo review, ligado no main.
type SecondReviews interface {
	PendingSecondReviews(ctx context.Context) ([]PendingSecondReview, error)
}

// TxRunner abre uma transação levada no contexto.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// AuditEvent é o que o módulo manda para a Auditoria.
type AuditEvent struct {
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

// AuditRecorder grava um evento de Auditoria na transação do contexto.
type AuditRecorder interface {
	Record(ctx context.Context, e AuditEvent) error
}

// ActionClaimRelease: Revisor ou Admin soltando a Posse de outra pessoa.
const (
	ActionClaimRelease = "claim.release"
	targetProposition  = "proposition"
)

func actor(ctx context.Context) (auth.Actor, error) {
	a, ok := auth.ActorFrom(ctx)
	if !ok {
		return auth.Actor{}, ErrUnauthenticated
	}
	if !a.Role.AtLeast(auth.RoleEditor) {
		return a, ErrForbidden
	}
	return a, nil
}

// ClaimView é a Posse que vale agora, com o nome de quem a tem.
type ClaimView struct {
	domain.Claim
	HolderName string
}

// current lê a Posse e diz se ela vale (não expirada, conta ativa).
func current(ctx context.Context, claims ClaimRepo, people People, propositionID string, now time.Time) (*domain.Claim, *ClaimView, error) {
	c, err := claims.Find(ctx, propositionID)
	if err != nil || c == nil {
		return c, nil, err
	}
	ps, err := people.Lookup(ctx, []string{c.UserID})
	if err != nil {
		return nil, nil, err
	}
	p := ps[c.UserID]
	if !c.Valid(now, p.Active) {
		return c, nil, nil
	}
	return c, &ClaimView{Claim: *c, HolderName: p.Name}, nil
}
