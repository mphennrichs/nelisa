// Package app tem os casos de uso da Proposição e dos Vínculos de fonte, a
// camada pública (regra de visibilidade) e as portas de que precisam. A
// autorização da equipe acontece aqui, com o Ator vindo do contexto
// (platform/auth); o handler só recusa cedo.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// PropositionRepo persiste Proposições. Usa a transação do contexto quando
// há. Erros de domínio: ErrPropositionNotFound (inclusive para id que não é
// UUID), ErrPropositionExists e ErrSlugTaken.
type PropositionRepo interface {
	Create(ctx context.Context, p domain.Proposition) error
	FindByID(ctx context.Context, id string) (domain.Proposition, error)
	// FindByIDForUpdate trava a linha até o fim da transação do contexto.
	FindByIDForUpdate(ctx context.Context, id string) (domain.Proposition, error)
	FindBySlug(ctx context.Context, slug string) (domain.Proposition, error)
	SlugExists(ctx context.Context, slug string) (bool, error)
	// FindByName devolve as Proposições com esse Tipo/número/ano (de
	// qualquer Casa iniciadora).
	FindByName(ctx context.Context, kind string, number, year int) ([]domain.Proposition, error)
	List(ctx context.Context) ([]domain.Proposition, error)
	ListInStates(ctx context.Context, states []domain.ReviewState) ([]domain.Proposition, error)
	ListAttachedTo(ctx context.Context, principalID string) ([]domain.Proposition, error)
	// ListUnresolvedAttachments devolve as apensadas a name cuja principal
	// ainda não foi resolvida.
	ListUnresolvedAttachments(ctx context.Context, name string) ([]domain.Proposition, error)
	// Update grava dados oficiais, Apensação, estado de Revisão e UpdatedAt.
	Update(ctx context.Context, p domain.Proposition) error
}

// LinkRepo persiste Vínculos de fonte. Erros de domínio: ErrLinkNotFound,
// ErrLinkTaken (o registro já está confirmado em outra Proposição),
// ErrChamberAlreadyLinked, ErrLinkExists e ErrLinkNotSuggested.
type LinkRepo interface {
	Create(ctx context.Context, l domain.SourceLink) error
	FindByID(ctx context.Context, id string) (domain.SourceLink, error)
	ListByProposition(ctx context.Context, propositionID string) ([]domain.SourceLink, error)
	ListByPropositions(ctx context.Context, propositionIDs []string) ([]domain.SourceLink, error)
	ListByExternal(ctx context.Context, chamber domain.Chamber, externalIDs []string) ([]domain.SourceLink, error)
	ListConfirmedIDs(ctx context.Context) ([]string, error)
	// Confirm grava a confirmação só se o vínculo ainda for sugestão.
	Confirm(ctx context.Context, l domain.SourceLink) error
	// DeleteSuggested apaga só se ainda for sugestão.
	DeleteSuggested(ctx context.Context, id string) error
	// UpdateSync grava o resultado da última Sincronização e o nome/link
	// atuais na Casa.
	UpdateSync(ctx context.Context, l domain.SourceLink) error
}

// TimelineEntry é um evento de Tramitação como a Proposição o mostra.
type TimelineEntry struct {
	Chamber     string
	At          time.Time
	Body        string
	Description string
	Situation   string
	Dispatch    string
	// Manual: evento de uma Atualização de tramitação aceita pela equipe,
	// com o link de fonte; ContributionID diz de qual Contribuição veio.
	Manual         bool
	SourceURL      string
	ContributionID string
}

// TimelineReader lê a Tramitação (módulo progress, ligado no main).
type TimelineReader interface {
	Timeline(ctx context.Context, propositionID string) ([]TimelineEntry, error)
}

// SyncScheduler enfileira a Sincronização de um Vínculo de fonte (River,
// ligado no main). Dentro de uma transação, o job entra nela.
type SyncScheduler interface {
	ScheduleSync(ctx context.Context, linkID string) error
}

// TxRunner abre uma transação levada no contexto: tudo o que os stores
// fizerem com esse contexto (inclusive a Auditoria e o enfileiramento) é
// gravado ou desfeito junto.
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
	ActionLinkConfirm     = "source_link.confirm"
	ActionLinkReject      = "source_link.reject"
	ActionSubmitForReview = "proposition.submit_for_review"

	targetProposition = "proposition"
	targetLink        = "source_link"
)

// newID gera um UUID v4.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("gerando id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
