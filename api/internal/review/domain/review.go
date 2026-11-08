// Package domain tem as regras da Revisão (CONTEXT.md): quem pode marcar
// uma Versão como pronta, os quatro olhos, a Segunda revisão pendente e o
// Selo de revisão. Só Go padrão.
package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
)

var (
	// ErrFourEyes: quem escreveu o conteúdo não o aprova, e quem aprovou
	// não faz a segunda revisão.
	ErrFourEyes = errors.New("review: quem escreveu ou aprovou este conteúdo não pode revisá-lo")
	// ErrNotInReview: só Proposição "em revisão" é aprovada; rascunho
	// precisa ser enviado para revisão antes.
	ErrNotInReview = errors.New("review: a proposição não está em revisão")
	// ErrNoSecondReviewPending: a aprovação teve quatro olhos ou a segunda
	// revisão já foi feita.
	ErrNoSecondReviewPending = errors.New("review: não há segunda revisão pendente")
	ErrApprovalNotFound      = errors.New("review: aprovação não encontrada")
	// ErrNotReviewer: só Revisor ou Admin marca como pronto ou faz a
	// segunda revisão.
	ErrNotReviewer = errors.New("review: só revisor ou admin pode revisar")
	// ErrNoPendingVersion: não há Versão em análise para aprovar.
	ErrNoPendingVersion = errors.New("review: não há versão em análise")
	// ErrAlreadyApproved: a Versão já tem aprovação.
	ErrAlreadyApproved = errors.New("review: versão já aprovada")
)

// ErrContentIncomplete: falta algo no conteúdo. Use errors.As com
// *ContentIncompleteError para saber o quê.
var ErrContentIncomplete = errors.New("review: conteúdo incompleto")

// ContentIncompleteError lista o que falta para aprovar.
type ContentIncompleteError struct{ Missing []Blocker }

func (e *ContentIncompleteError) Error() string {
	msgs := make([]string, 0, len(e.Missing))
	for _, m := range e.Missing {
		msgs = append(msgs, m.Message)
	}
	return "review: conteúdo incompleto: " + strings.Join(msgs, " ")
}

func (e *ContentIncompleteError) Is(target error) bool { return target == ErrContentIncomplete }

// Blocker é um motivo pelo qual a aprovação não pode acontecer agora.
type Blocker struct {
	Code    string
	Message string
}

// Códigos dos impedimentos que são da revisão (os do conteúdo vêm do
// módulo proposition, com os próprios códigos).
const (
	BlockerRole           = "role"
	BlockerNoPending      = "no_pending_version"
	BlockerNotInReview    = "not_in_review"
	BlockerFourEyes       = "four_eyes"
	blockerMessageRole    = "Só Revisor ou Admin marca como pronto."
	blockerMessageNone    = "Não há Versão em análise."
	blockerMessageState   = "Envie a Proposição para revisão antes de aprovar."
	blockerMessageAuthors = "Você escreveu este conteúdo; outra pessoa precisa aprovar (quatro olhos)."
)

// Approver é quem tenta aprovar, com o que a role dele permite.
type Approver struct {
	UserID string
	// CanReview: Revisor ou Admin.
	CanReview bool
	// IsAdmin: o único que pode aprovar o próprio trabalho.
	IsAdmin bool
}

// Candidate é a Versão em análise como a revisão a enxerga.
type Candidate struct {
	// HasPending: existe Versão em análise.
	HasPending bool
	InReview   bool
	Authors    []string
	// Missing é o que falta no conteúdo (vem do módulo proposition).
	Missing []Blocker
}

// Assessment é a decisão: pode aprovar se não há impedimentos.
// BreaksFourEyes: o Admin autor aprovando; fica a Segunda revisão
// pendente.
type Assessment struct {
	Blockers       []Blocker
	BreaksFourEyes bool
}

// CanApprove diz se a aprovação pode acontecer.
func (a Assessment) CanApprove() bool { return len(a.Blockers) == 0 }

// Assess aplica as regras de aprovação (marcar pronto):
//  1. Revisor ou Admin;
//  2. Versão em análise numa Proposição "em revisão";
//  3. quatro olhos: autor não aprova, exceto o Admin (sem quatro olhos);
//  4. conteúdo completo.
func Assess(c Candidate, who Approver) Assessment {
	var out Assessment
	if !who.CanReview {
		out.Blockers = append(out.Blockers, Blocker{BlockerRole, blockerMessageRole})
	}
	if !c.HasPending {
		out.Blockers = append(out.Blockers, Blocker{BlockerNoPending, blockerMessageNone})
		return out
	}
	if !c.InReview {
		out.Blockers = append(out.Blockers, Blocker{BlockerNotInReview, blockerMessageState})
	}
	if slices.Contains(c.Authors, who.UserID) {
		if who.IsAdmin {
			out.BreaksFourEyes = true
		} else {
			out.Blockers = append(out.Blockers, Blocker{BlockerFourEyes, blockerMessageAuthors})
		}
	}
	out.Blockers = append(out.Blockers, c.Missing...)
	return out
}

// Err traduz a avaliação no erro da tentativa de aprovar, do mais
// fundamental para o mais circunstancial.
func (a Assessment) Err() error {
	var missing []Blocker
	for _, b := range a.Blockers {
		switch b.Code {
		case BlockerRole:
			return ErrNotReviewer
		case BlockerNoPending:
			return ErrNoPendingVersion
		case BlockerNotInReview:
			return ErrNotInReview
		case BlockerFourEyes:
			return ErrFourEyes
		default:
			missing = append(missing, b)
		}
	}
	if len(missing) > 0 {
		return &ContentIncompleteError{Missing: missing}
	}
	return nil
}

// Approval é a aprovação (marcar pronto) de uma Versão.
type Approval struct {
	VersionID     string
	PropositionID string
	ApprovedBy    string
	ApprovedAt    time.Time
	// FourEyes: aprovada por quem não escreveu. Falso só quando o Admin
	// aprovou o próprio trabalho.
	FourEyes       bool
	SecondReviewBy string
	SecondReviewAt *time.Time
}

// NewApproval registra a aprovação depois de Assess sem impedimentos.
func NewApproval(propositionID, versionID string, a Assessment, approverID string, now time.Time) Approval {
	return Approval{
		VersionID: versionID, PropositionID: propositionID, ApprovedBy: approverID, ApprovedAt: now,
		FourEyes: !a.BreaksFourEyes,
	}
}

// PendingSecondReview: aprovada sem quatro olhos e ainda sem segunda
// revisão.
func (a Approval) PendingSecondReview() bool { return !a.FourEyes && a.SecondReviewBy == "" }

// Badge é o Selo de revisão: 2 pessoas com quatro olhos ou segunda
// revisão feita; senão 1.
func (a Approval) Badge() int {
	if a.FourEyes || a.SecondReviewBy != "" {
		return 2
	}
	return 1
}

// CanSecondReview diz se reviewer pode confirmar a segunda revisão:
// Revisor ou Admin que não está na Autoria e não é quem aprovou.
func (a Approval) CanSecondReview(reviewer Approver, authors []string) error {
	if !reviewer.CanReview {
		return ErrNotReviewer
	}
	if !a.PendingSecondReview() {
		return ErrNoSecondReviewPending
	}
	if reviewer.UserID == a.ApprovedBy || slices.Contains(authors, reviewer.UserID) {
		return ErrFourEyes
	}
	return nil
}

// ConfirmSecondReview grava a segunda revisão.
func (a *Approval) ConfirmSecondReview(reviewer Approver, authors []string, now time.Time) error {
	if err := a.CanSecondReview(reviewer, authors); err != nil {
		return err
	}
	a.SecondReviewBy, a.SecondReviewAt = reviewer.UserID, &now
	return nil
}
