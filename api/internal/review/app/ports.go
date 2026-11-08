// Package app tem os casos de uso da Revisão (aprovar, segunda revisão) e
// as consultas que os outros módulos fazem a ela (Selo, situação para a
// equipe, Segundas revisões pendentes para a Fila), com as portas de que
// precisam. A autorização acontece aqui, com o Ator vindo do contexto.
package app

import (
	"context"
	"errors"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

var (
	ErrUnauthenticated = errors.New("review: não autenticado")
	ErrForbidden       = errors.New("review: sem permissão")
	// ErrPropositionNotFound e ErrVersionNotFound vêm do módulo
	// proposition, traduzidos no main.
	ErrPropositionNotFound = errors.New("review: proposição não encontrada")
	ErrVersionNotFound     = errors.New("review: versão não encontrada")
	// ErrVersionNotPending: a Versão não está mais em análise (já foi
	// aprovada, ou outra a substituiu).
	ErrVersionNotPending = errors.New("review: a versão não está em análise")
)

// ApprovalRepo persiste aprovações. Create devolve
// domain.ErrAlreadyApproved se a Versão já tem uma; os Find,
// domain.ErrApprovalNotFound.
type ApprovalRepo interface {
	Create(ctx context.Context, a domain.Approval) error
	FindByVersion(ctx context.Context, versionID string) (domain.Approval, error)
	// FindByVersionForUpdate trava a aprovação até o fim da transação.
	FindByVersionForUpdate(ctx context.Context, versionID string) (domain.Approval, error)
	ListByVersions(ctx context.Context, versionIDs []string) ([]domain.Approval, error)
	ListPendingSecondReview(ctx context.Context) ([]domain.Approval, error)
	// SetSecondReview grava a segunda revisão só se ela ainda estiver
	// pendente; senão domain.ErrNoSecondReviewPending.
	SetSecondReview(ctx context.Context, a domain.Approval) error
}

// Subject é a Versão em análise de uma Proposição, como o módulo
// proposition a descreve: estado, Autoria e o que falta no conteúdo.
type Subject struct {
	PropositionID string
	Name          string
	InReview      bool
	VersionID     string
	VersionSeq    int
	// Revision do conteúdo: aprovar exige ter lido esta.
	Revision int
	Authors  []string
	Missing  []domain.Blocker
}

// VersionInfo é uma Versão aprovada e a Autoria do conteúdo dela.
type VersionInfo struct {
	PropositionID string
	VersionID     string
	Seq           int
	Current       bool
	Authors       []string
}

// Editorial é o ciclo editorial (módulo proposition, ligado no main).
// Erros: ErrPropositionNotFound, ErrVersionNotFound, ErrVersionNotPending
// e domain.ErrNotInReview.
type Editorial interface {
	// Pending devolve a Versão em análise, ou nil se não há.
	Pending(ctx context.Context, propositionID string) (*Subject, error)
	// LockPending trava a Proposição até o fim da transação e devolve a
	// Versão pedida, que tem de estar em análise.
	LockPending(ctx context.Context, propositionID, versionID string) (Subject, error)
	// Promote: a Versão vira a vigente, a anterior é substituída e a
	// Proposição fica pronta.
	Promote(ctx context.Context, propositionID, versionID string) error
	Version(ctx context.Context, propositionID, versionID string) (VersionInfo, error)
	// CurrentVersionID devolve a vigente ("" se não há).
	CurrentVersionID(ctx context.Context, propositionID string) (string, error)
}

// Claims é a Posse (módulo queue, ligado no main). Aprovar solta a Posse.
type Claims interface {
	Release(ctx context.Context, propositionID string) error
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

// Ações auditadas.
const (
	ActionApprove      = "review.approve"
	ActionSecondReview = "review.second_review"

	targetVersion = "version"
)

// approver lê o Ator do contexto. Qualquer pessoa da equipe é avaliada;
// quem não é Revisor leva o impedimento de role.
func approver(ctx context.Context) (domain.Approver, error) {
	a, ok := auth.ActorFrom(ctx)
	if !ok {
		return domain.Approver{}, ErrUnauthenticated
	}
	if !a.Role.AtLeast(auth.RoleEditor) {
		return domain.Approver{}, ErrForbidden
	}
	return domain.Approver{UserID: a.UserID, CanReview: a.Role.AtLeast(auth.RoleReviewer), IsAdmin: a.Role.AtLeast(auth.RoleAdmin)}, nil
}

// requireReviewer recusa quem não é Revisor ou Admin.
func requireReviewer(ctx context.Context) (domain.Approver, error) {
	who, err := approver(ctx)
	if err != nil {
		return who, err
	}
	if !who.CanReview {
		return who, ErrForbidden
	}
	return who, nil
}
