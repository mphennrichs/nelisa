package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Correção menor e Errata (CONTEXT.md): o Nelisa corrigindo o próprio
// conteúdo já publicado (Resumo, Resumo curto, Temas da Versão vigente).
// A Correção menor não muda o sentido e entra direto, ficando "a conferir";
// a Errata muda o sentido e só entra com quatro olhos, virando nota
// pública datada. As duas só valem numa Proposição pronta: é o conteúdo
// que o Leitor já vê.

var (
	// ErrNotReady: Correção menor e Errata são de Proposição pronta.
	ErrNotReady = errors.New("review: a proposição não está pronta")
	// ErrErratumPending: já há uma Errata pendente na Versão; outra
	// Errata ou uma Correção menor mexeriam no mesmo texto.
	ErrErratumPending = errors.New("review: há uma errata pendente nesta versão")
	// ErrNoChanges: a correção não muda nada no conteúdo.
	ErrNoChanges = errors.New("review: a correção não muda o conteúdo")
	// ErrInvalidErratumNote: "dizia" e "o correto é" são obrigatórios,
	// até 500 caracteres cada.
	ErrInvalidErratumNote = errors.New("review: nota da errata inválida")
	// ErrInvalidReason: recusar exige um motivo (interno), até 1.000
	// caracteres.
	ErrInvalidReason      = errors.New("review: motivo da recusa inválido")
	ErrMinorFixNotFound   = errors.New("review: correção menor não encontrada")
	ErrMinorFixChecked    = errors.New("review: correção menor já conferida")
	ErrErratumNotFound    = errors.New("review: errata não encontrada")
	ErrErratumNotPending  = errors.New("review: a errata não está pendente")
	ErrNotErratumProposer = errors.New("review: só quem propôs pode retirar a errata")
	ErrVersionNotCurrent  = errors.New("review: só a versão vigente recebe correção menor ou errata")
)

// Limites da nota pública e do motivo interno.
const (
	MaxErratumNoteLen = 500
	MaxReasonLen      = 1000
)

// Snapshot é o conteúdo editorial inteiro de uma Versão, como a revisão o
// guarda: o antes e o depois de uma Correção menor, o proposto de uma
// Errata.
type Snapshot struct {
	Summary      string
	ShortSummary string
	TopicIDs     []string
}

// Equal compara dois conteúdos (Temas na ordem em que foram escolhidos).
func (s Snapshot) Equal(o Snapshot) bool {
	return s.Summary == o.Summary && s.ShortSummary == o.ShortSummary &&
		slices.Equal(nonNil(s.TopicIDs), nonNil(o.TopicIDs))
}

// Target é a Versão que vai ser corrigida, como a revisão a enxerga.
type Target struct {
	PropositionID string
	Name          string
	// Ready: a Proposição está pronta.
	Ready     bool
	VersionID string
	Seq       int
	Content   Snapshot
	Revision  int
}

// CanCorrect diz se a Versão aceita Correção menor ou Errata agora:
// Proposição pronta e nenhuma Errata pendente na Versão.
func (t Target) CanCorrect(erratumPending bool) error {
	if !t.Ready {
		return ErrNotReady
	}
	if erratumPending {
		return ErrErratumPending
	}
	return nil
}

// MinorFix é uma Correção menor aplicada à Versão vigente.
type MinorFix struct {
	ID            string
	PropositionID string
	VersionID     string
	// Fields são os campos que mudaram ("summary", "short_summary",
	// "topic_ids").
	Fields    []string
	Before    Snapshot
	After     Snapshot
	CreatedBy string
	CreatedAt time.Time
	CheckedBy string
	CheckedAt *time.Time
}

// NewMinorFix registra a correção já aplicada. Sem campo mudado não há
// correção.
func NewMinorFix(id string, t Target, fields []string, after Snapshot, by string, now time.Time) (MinorFix, error) {
	if len(fields) == 0 {
		return MinorFix{}, ErrNoChanges
	}
	return MinorFix{
		ID: id, PropositionID: t.PropositionID, VersionID: t.VersionID, Fields: slices.Clone(fields),
		Before: t.Content, After: after, CreatedBy: by, CreatedAt: now,
	}, nil
}

// Unchecked: ainda "a conferir".
func (f MinorFix) Unchecked() bool { return f.CheckedBy == "" }

// CanCheck: Revisor ou Admin que não fez a correção. Sem exceção para o
// Admin: conferir é o segundo par de olhos sobre um texto já publicado.
func (f MinorFix) CanCheck(who Approver) error {
	if !who.CanReview {
		return ErrNotReviewer
	}
	if !f.Unchecked() {
		return ErrMinorFixChecked
	}
	if who.UserID == f.CreatedBy {
		return ErrFourEyes
	}
	return nil
}

// Check grava a conferência.
func (f *MinorFix) Check(who Approver, now time.Time) error {
	if err := f.CanCheck(who); err != nil {
		return err
	}
	f.CheckedBy, f.CheckedAt = who.UserID, &now
	return nil
}

// ErratumState: pendente → aprovada, recusada ou retirada.
type ErratumState string

const (
	ErratumPending   ErratumState = "pending"
	ErratumApproved  ErratumState = "approved"
	ErratumRejected  ErratumState = "rejected"
	ErratumWithdrawn ErratumState = "withdrawn"
)

// Erratum é uma Errata proposta para a Versão vigente.
type Erratum struct {
	ID            string
	PropositionID string
	VersionID     string
	// Proposed é o conteúdo completo como fica depois da Errata.
	Proposed Snapshot
	// Said e Correct são a nota pública: "dizia X; o correto é Y".
	Said    string
	Correct string
	// BaseRevision é a revision do conteúdo vigente quando a Errata foi
	// proposta.
	BaseRevision int
	State        ErratumState
	CreatedBy    string
	CreatedAt    time.Time
	DecidedBy    string
	DecidedAt    *time.Time
	// Reason é o motivo interno da recusa.
	Reason string
}

// normalizeText tira espaços das pontas e unifica quebras de linha.
func normalizeText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func validNote(s string) bool {
	return s != "" && utf8.RuneCountInString(s) <= MaxErratumNoteLen
}

// NewErratum propõe uma Errata sobre o conteúdo vigente t. proposed já
// vem validado pelo conteúdo (módulo proposition); aqui, a nota e se há
// mudança de fato.
func NewErratum(id string, t Target, proposed Snapshot, said, correct, by string, now time.Time) (Erratum, error) {
	said, correct = normalizeText(said), normalizeText(correct)
	if !validNote(said) || !validNote(correct) {
		return Erratum{}, ErrInvalidErratumNote
	}
	if proposed.Equal(t.Content) {
		// Errata que não muda o texto seria só uma nota solta: o Leitor
		// leria "o correto é Y" sobre um conteúdo que já diz Y.
		return Erratum{}, ErrNoChanges
	}
	return Erratum{
		ID: id, PropositionID: t.PropositionID, VersionID: t.VersionID, Proposed: proposed,
		Said: said, Correct: correct, BaseRevision: t.Revision, State: ErratumPending,
		CreatedBy: by, CreatedAt: now,
	}, nil
}

// Pending: ainda sem decisão.
func (e Erratum) Pending() bool { return e.State == ErratumPending }

// Stale diz se o conteúdo vigente mudou desde a proposta (revision é a
// atual): aprovar aplicaria a Errata sobre um texto que ela não viu.
func (e Erratum) Stale(revision int) bool { return revision != e.BaseRevision }

// CanDecide: Revisor ou Admin, nunca quem propôs. Sem exceção para o
// Admin: a Errata corrige um erro já publicado, e o CONTEXT.md pede
// "outro Revisor".
func (e Erratum) CanDecide(who Approver) error {
	if !who.CanReview {
		return ErrNotReviewer
	}
	if !e.Pending() {
		return ErrErratumNotPending
	}
	if who.UserID == e.CreatedBy {
		return ErrFourEyes
	}
	return nil
}

// Approve aprova a Errata. Conferir se o conteúdo base não mudou é de
// quem chama (precisa da revision atual).
func (e *Erratum) Approve(who Approver, now time.Time) error {
	if err := e.CanDecide(who); err != nil {
		return err
	}
	e.State, e.DecidedBy, e.DecidedAt = ErratumApproved, who.UserID, &now
	return nil
}

// Reject recusa a Errata com um motivo interno.
func (e *Erratum) Reject(who Approver, reason string, now time.Time) error {
	if err := e.CanDecide(who); err != nil {
		return err
	}
	reason = normalizeText(reason)
	if reason == "" || utf8.RuneCountInString(reason) > MaxReasonLen {
		return ErrInvalidReason
	}
	e.State, e.DecidedBy, e.DecidedAt, e.Reason = ErratumRejected, who.UserID, &now, reason
	return nil
}

// CanWithdraw: só quem propôs, enquanto pendente.
func (e Erratum) CanWithdraw(userID string) error {
	if !e.Pending() {
		return ErrErratumNotPending
	}
	if userID != e.CreatedBy {
		return ErrNotErratumProposer
	}
	return nil
}

// Withdraw é quem propôs desistindo.
func (e *Erratum) Withdraw(userID string, now time.Time) error {
	if err := e.CanWithdraw(userID); err != nil {
		return err
	}
	e.State, e.DecidedBy, e.DecidedAt = ErratumWithdrawn, userID, &now
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
