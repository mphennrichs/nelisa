// Package app tem os casos de uso das Contribuições e da Fonte oficial e
// as portas de que precisam. O envio e a consulta são do Leitor, sem login;
// o cadastro de Fonte oficial é da equipe, com a autorização aqui e o Ator
// vindo do contexto.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
)

var (
	ErrUnauthenticated = errors.New("contribution: não autenticado")
	ErrForbidden       = errors.New("contribution: sem permissão")
	// ErrTrackingCodeTaken: o código sorteado já existe (60 bits: na
	// prática nunca). O envio sorteia de novo.
	ErrTrackingCodeTaken = errors.New("contribution: código de acompanhamento repetido")
)

// ContributionRepo persiste as Contribuições.
type ContributionRepo interface {
	// Insert grava; ErrTrackingCodeTaken se o hash do código já existe.
	Insert(ctx context.Context, c domain.Contribution) error
	// FindByTrackingHash devolve ErrContributionNotFound se não há.
	FindByTrackingHash(ctx context.Context, hash string) (domain.Contribution, error)
	CountPending(ctx context.Context) (int, error)
	// Find devolve ErrContributionNotFound se não há (ou o id não é UUID).
	Find(ctx context.Context, id string) (domain.Contribution, error)
	// FindForUpdate é Find travando a linha até o fim da transação do
	// contexto: duas decisões ao mesmo tempo não passam.
	FindForUpdate(ctx context.Context, id string) (domain.Contribution, error)
	// ListPending lista as pendentes, das mais antigas para as mais novas.
	ListPending(ctx context.Context) ([]domain.Contribution, error)
	// Update grava a moderação: estado, Esfera, decisão, motivo, nota e
	// Proposição criada.
	Update(ctx context.Context, c domain.Contribution) error
}

// OfficialSourceRepo persiste a Fonte oficial.
type OfficialSourceRepo interface {
	List(ctx context.Context) ([]domain.OfficialSource, error)
	Domains(ctx context.Context) ([]string, error)
	// Insert devolve ErrOfficialSourceExists se o domínio já está
	// cadastrado.
	Insert(ctx context.Context, s domain.OfficialSource) error
	// Delete devolve o cadastro apagado, ou ErrOfficialSourceNotFound.
	Delete(ctx context.Context, id string) (domain.OfficialSource, error)
}

// PublicProposition é uma Proposição que o Leitor vê.
type PublicProposition struct {
	ID     string
	Slug   string
	Sphere domain.Sphere
}

// Propositions é o módulo proposition, ligado no main. Só enxerga
// Proposições públicas (a regra de visibilidade é dele).
type Propositions interface {
	// PublicBySlug devolve nil se a Proposição não existe ou não é pública.
	PublicBySlug(ctx context.Context, slug string) (*PublicProposition, error)
	// PublicByID idem, pelo id.
	PublicByID(ctx context.Context, id string) (*PublicProposition, error)
	// ByID devolve a Proposição para a equipe, pública ou não; nil se não
	// existe.
	ByID(ctx context.Context, id string) (*TeamProposition, error)
}

// TeamProposition é uma Proposição como a equipe a vê na moderação.
type TeamProposition struct {
	ID     string
	Name   string
	Slug   string
	Sphere domain.Sphere
}

// ManualEvent é o evento de Tramitação de uma Atualização aceita.
type ManualEvent struct {
	PropositionID  string
	ContributionID string
	domain.ProgressEvent
}

// ProgressEntries é a Tramitação (módulo progress, ligado no main).
type ProgressEntries interface {
	// RecordManual grava o evento manual; ErrProgressEntryExists se a
	// Contribuição já gravou o dela.
	RecordManual(ctx context.Context, e ManualEvent) error
	// DeleteManual apaga o evento manual da Contribuição; false se não há.
	DeleteManual(ctx context.Context, contributionID string) (bool, error)
}

// ErrProgressEntryExists: a Contribuição já tem evento manual na
// Tramitação.
var ErrProgressEntryExists = errors.New("contribution: evento manual já gravado")

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

const (
	ActionOfficialSourceCreate = "official_source.create"
	ActionOfficialSourceDelete = "official_source.delete"
	targetOfficialSource       = "official_source"

	ActionContributionAccept         = "contribution.accept"
	ActionContributionReject         = "contribution.reject"
	ActionContributionSphere         = "contribution.sphere_change"
	ActionContributionLink           = "contribution.link_proposition"
	ActionContributionProgressDelete = "contribution.progress_entry_delete"
	targetContribution               = "contribution"
)

func requireRole(ctx context.Context, min auth.Role) (auth.Actor, error) {
	a, ok := auth.ActorFrom(ctx)
	if !ok {
		return auth.Actor{}, ErrUnauthenticated
	}
	if !a.Role.AtLeast(min) {
		return a, ErrForbidden
	}
	return a, nil
}

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
