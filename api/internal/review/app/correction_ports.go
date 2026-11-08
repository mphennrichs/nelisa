package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

// Portas da Correção menor e da Errata.

// MinorFixRepo persiste Correções menores. Os Find devolvem
// domain.ErrMinorFixNotFound (inclusive para id que não é UUID).
type MinorFixRepo interface {
	Create(ctx context.Context, f domain.MinorFix) error
	// FindForUpdate trava a correção até o fim da transação.
	FindForUpdate(ctx context.Context, id string) (domain.MinorFix, error)
	// SetChecked grava a conferência só se ainda não houver uma; senão
	// domain.ErrMinorFixChecked.
	SetChecked(ctx context.Context, f domain.MinorFix) error
	ListUncheckedByProposition(ctx context.Context, propositionID string) ([]domain.MinorFix, error)
	// ListUnchecked lista todas as correções a conferir, da mais antiga
	// para a mais nova (Fila).
	ListUnchecked(ctx context.Context) ([]domain.MinorFix, error)
}

// ErratumRepo persiste Erratas. Create devolve domain.ErrErratumPending se
// a Versão já tem uma pendente; os Find, domain.ErrErratumNotFound.
type ErratumRepo interface {
	Create(ctx context.Context, e domain.Erratum) error
	FindForUpdate(ctx context.Context, id string) (domain.Erratum, error)
	// Decide grava a decisão só se a Errata ainda estiver pendente; senão
	// domain.ErrErratumNotPending.
	Decide(ctx context.Context, e domain.Erratum) error
	HasPending(ctx context.Context, versionID string) (bool, error)
	// ListByProposition lista todas, da mais nova para a mais antiga.
	ListByProposition(ctx context.Context, propositionID string) ([]domain.Erratum, error)
	// ListPending lista as pendentes de todas as Proposições, da mais
	// antiga para a mais nova (Fila).
	ListPending(ctx context.Context) ([]domain.Erratum, error)
	// ListApprovedByVersions lista as aprovadas das Versões pedidas, da
	// mais nova para a mais antiga (camada pública).
	ListApprovedByVersions(ctx context.Context, versionIDs []string) ([]domain.Erratum, error)
}

// InvalidContentError é o conteúdo proposto recusado pelas validações do
// conteúdo (módulo proposition, traduzido no main): Code e Message vão
// para a equipe.
type InvalidContentError struct {
	Code    string
	Message string
}

func (e *InvalidContentError) Error() string { return "review: conteúdo inválido: " + e.Message }

// ErrClaimTaken: outra pessoa tem a Posse. Use errors.As com
// *ClaimTakenError para saber quem.
var ErrClaimTaken = errors.New("review: outra pessoa está com a posse")

// ClaimTakenError diz quem tem a Posse (nome só para a equipe).
type ClaimTakenError struct{ HolderName string }

func (e *ClaimTakenError) Error() string { return "review: " + e.HolderName + " está com a posse" }

func (e *ClaimTakenError) Is(target error) bool { return target == ErrClaimTaken }

// Rewrite é o resultado de trocar o conteúdo vigente.
type Rewrite struct {
	// Fields são os campos que mudaram (vazio: nada foi gravado).
	Fields []string
	// Content e Revision são o conteúdo como ficou.
	Content  domain.Snapshot
	Revision int
}

// Corrections é o que a Correção menor e a Errata pedem ao módulo
// proposition (ligado no main). Erros: ErrPropositionNotFound,
// ErrVersionNotFound, domain.ErrVersionNotCurrent, domain.ErrStaleContent
// e *InvalidContentError.
type Corrections interface {
	// Current devolve a Versão vigente e o conteúdo dela, ou nil se não há.
	Current(ctx context.Context, propositionID string) (*domain.Target, error)
	// LockCurrent trava a Proposição até o fim da transação e devolve a
	// Versão pedida, que tem de ser a vigente.
	LockCurrent(ctx context.Context, propositionID, versionID string) (domain.Target, error)
	// CheckContent aplica ao conteúdo proposto as mesmas validações da
	// gravação do conteúdo, mais as do conteúdo publicado (Resumo, Resumo
	// curto e 1 a 3 Temas), e devolve o conteúdo normalizado. previous são
	// os Temas que a Versão já usa (um Tema desativado depois continua
	// valendo para ela).
	CheckContent(ctx context.Context, proposed domain.Snapshot, previous []string) (domain.Snapshot, error)
	// Rewrite troca o conteúdo da Versão vigente por proposed, com
	// authorID entrando na Autoria, se a revision guardada ainda for
	// revision. Chame dentro da transação de LockCurrent. Sem mudança, nada
	// é gravado.
	Rewrite(ctx context.Context, propositionID, versionID string, proposed domain.Snapshot, authorID string, revision int) (Rewrite, error)
}

// ClaimGate é a Posse para a Correção menor (módulo queue, ligado no
// main).
type ClaimGate interface {
	// TouchForEdit assume a Posse se livre, renova se já é de userID e
	// devolve *ClaimTakenError se é de outra pessoa.
	TouchForEdit(ctx context.Context, propositionID, userID string) error
}

// Ações auditadas da Correção menor e da Errata. A Correção menor é uma
// mudança de conteúdo (content.*); o resto é revisão (review.*).
const (
	ActionMinorFix        = "content.minor_fix"
	ActionMinorFixCheck   = "review.minor_fix_check"
	ActionErratumPropose  = "review.erratum_propose"
	ActionErratumApprove  = "review.erratum_approve"
	ActionErratumReject   = "review.erratum_reject"
	ActionErratumWithdraw = "review.erratum_withdraw"

	targetMinorFix = "minor_fix"
	targetErratum  = "erratum"
)

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
