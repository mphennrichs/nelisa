package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// VersionRepo persiste Versões. Erros de domínio: ErrVersionNotFound
// (inclusive para id que não é UUID) e ErrPendingVersionExists (o índice
// único de uma Versão em análise por Proposição).
type VersionRepo interface {
	Create(ctx context.Context, v domain.Version) error
	FindByID(ctx context.Context, id string) (domain.Version, error)
	ListByProposition(ctx context.Context, propositionID string) ([]domain.Version, error)
	ListByPropositions(ctx context.Context, propositionIDs []string) ([]domain.Version, error)
	// UpdateState muda o estado só se ainda for from; senão
	// ErrVersionNotPending (a Versão mudou no meio do caminho).
	UpdateState(ctx context.Context, id string, from, to domain.VersionState) error
}

// ContentRepo persiste o conteúdo editorial (1:1 com a Versão).
type ContentRepo interface {
	Create(ctx context.Context, c domain.Content) error
	// FindByVersion devolve ErrVersionNotFound se não há conteúdo.
	FindByVersion(ctx context.Context, versionID string) (domain.Content, error)
	ListByVersions(ctx context.Context, versionIDs []string) ([]domain.Content, error)
	// Update grava c só se a revisão guardada ainda for prevRevision;
	// senão ErrStaleContent.
	Update(ctx context.Context, c domain.Content, prevRevision int) error
}

// TopicRepo persiste Temas. Erros: ErrTopicNotFound e ErrTopicNameTaken.
type TopicRepo interface {
	Create(ctx context.Context, t domain.Topic) error
	FindByID(ctx context.Context, id string) (domain.Topic, error)
	List(ctx context.Context) ([]domain.Topic, error)
	Update(ctx context.Context, t domain.Topic) error
}

// ErrClaimTaken: outra pessoa tem a Posse da Proposição. Use
// errors.As com *ClaimTakenError para saber quem.
var ErrClaimTaken = errors.New("proposition: outra pessoa está com a posse")

// ClaimTakenError diz quem tem a Posse (o nome vai para a equipe, nunca
// para o Leitor).
type ClaimTakenError struct{ HolderName string }

func (e *ClaimTakenError) Error() string {
	return fmt.Sprintf("proposition: %s está com a posse", e.HolderName)
}

func (e *ClaimTakenError) Is(target error) bool { return target == ErrClaimTaken }

// ClaimInfo é a Posse vigente de uma Proposição, para a equipe.
type ClaimInfo struct {
	UserID         string
	HolderName     string
	ClaimedAt      time.Time
	LastActivityAt time.Time
	ExpiresAt      time.Time
}

// Claims é a Posse (módulo queue, ligado no main).
type Claims interface {
	// TouchForEdit é quem grava conteúdo: assume a Posse se estiver livre
	// (ou expirada, ou de conta desativada), renova a atividade se já for
	// de userID e devolve *ClaimTakenError se for de outra pessoa. Roda na
	// transação do contexto.
	TouchForEdit(ctx context.Context, propositionID, userID string) error
	// Current devolve a Posse que vale agora, ou nil se está livre.
	Current(ctx context.Context, propositionID string) (*ClaimInfo, error)
}

// ReviewBlocker é um motivo pelo qual o Ator não pode aprovar.
type ReviewBlocker struct {
	Code    string
	Message string
}

// ReviewStatus é o que o módulo review diz de uma Proposição para o Ator
// do contexto: se ele pode aprovar a Versão em análise (e por que não),
// se pode fazer a segunda revisão da vigente, e a aprovação da vigente.
type ReviewStatus struct {
	CanApprove bool
	Blockers   []ReviewBlocker
	// ApprovalBreaksFourEyes: o Ator é Admin e autor; se aprovar, fica a
	// Segunda revisão pendente.
	ApprovalBreaksFourEyes bool
	CanSecondReview        bool
	Current                *ApprovalInfo
}

// ApprovalInfo é a aprovação de uma Versão, como a equipe a vê.
type ApprovalInfo struct {
	VersionID      string
	ApprovedBy     string
	ApprovedAt     time.Time
	FourEyes       bool
	SecondReviewBy string
	SecondReviewAt *time.Time
	// Badge é o Selo: 2 ou 1.
	Badge int
	// PendingSecondReview: aprovada sem quatro olhos e ainda sem a segunda
	// revisão.
	PendingSecondReview bool
}

// Reviews é o módulo review para a equipe, ligado no main.
type Reviews interface {
	// Status avalia a Proposição para o Ator do contexto.
	Status(ctx context.Context, propositionID string) (ReviewStatus, error)
}

// BadgeReader dá o Selo à camada pública (módulo review, ligado no main).
type BadgeReader interface {
	// Badges devolve o Selo (1 ou 2) de cada Versão aprovada entre as
	// pedidas; Versão sem aprovação fica de fora.
	Badges(ctx context.Context, versionIDs []string) (map[string]int, error)
}

// People traduz ids da equipe em nomes (módulo team, ligado no main).
// Id desconhecido fica de fora.
type People interface {
	Names(ctx context.Context, ids []string) (map[string]string, error)
}

// Ações auditadas do ciclo editorial.
const (
	ActionContentUpdate   = "content.update"
	ActionContentConfirm  = "content.confirm"
	ActionVersionCreate   = "version.create"
	ActionTopicCreate     = "topic.create"
	ActionTopicRename     = "topic.rename"
	ActionTopicDeactivate = "topic.deactivate"
	ActionTopicReactivate = "topic.reactivate"

	targetVersion = "version"
	targetTopic   = "topic"
)
