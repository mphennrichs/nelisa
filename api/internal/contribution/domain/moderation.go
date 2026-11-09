package domain

import (
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// RejectReason é o motivo público de uma rejeição, de uma lista fechada:
// o Leitor vê o motivo ao consultar o código.
type RejectReason string

const (
	ReasonSourceNotConfirmed RejectReason = "source_not_confirmed"
	ReasonOutOfScope         RejectReason = "out_of_scope"
	ReasonAlreadyRegistered  RejectReason = "already_registered"
	ReasonDuplicate          RejectReason = "duplicate"
	ReasonIncorrect          RejectReason = "incorrect"
	ReasonOther              RejectReason = "other"
)

// MaxInternalNoteLen limita a nota interna da equipe.
const MaxInternalNoteLen = 1000

// MaxEventDescriptionLen limita o texto do evento manual de Tramitação.
const MaxEventDescriptionLen = 1000

var (
	// ErrNotPending: a Contribuição já foi decidida.
	ErrNotPending      = errors.New("contribution: a contribuição já foi decidida")
	ErrInvalidReason   = errors.New("contribution: motivo de rejeição inválido")
	ErrInternalNoteLen = errors.New("contribution: nota interna longa demais")
	// ErrSphereFixed: a Esfera da Atualização de tramitação vem da
	// Proposição e não se corrige.
	ErrSphereFixed = errors.New("contribution: a esfera desta contribuição vem da proposição")
	// ErrNotLinkable: só a Indicação aceita, ainda sem Proposição, aponta
	// a Proposição criada.
	ErrNotLinkable = errors.New("contribution: só a indicação aceita, ainda sem proposição, aponta a proposição criada")
	// ErrInvalidChamber: Casa fora de Chambers.
	ErrInvalidChamber = errors.New("contribution: casa inválida")
	// ErrInvalidEventText: texto do evento vazio ou longo demais.
	ErrInvalidEventText = errors.New("contribution: descreva o evento")
	// ErrNoProgressEntry: a Contribuição não é uma Atualização de
	// tramitação aceita, ou o evento manual já foi apagado.
	ErrNoProgressEntry = errors.New("contribution: não há evento manual desta contribuição")
)

func (r RejectReason) Validate() error {
	switch r {
	case ReasonSourceNotConfirmed, ReasonOutOfScope, ReasonAlreadyRegistered, ReasonDuplicate, ReasonIncorrect, ReasonOther:
		return nil
	}
	return ErrInvalidReason
}

func normalizeInternalNote(s string) (string, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if utf8.RuneCountInString(s) > MaxInternalNoteLen {
		return "", ErrInternalNoteLen
	}
	return s, nil
}

func (c *Contribution) decide(state State, by, note string, now time.Time) error {
	if c.State != StatePending {
		return ErrNotPending
	}
	n, err := normalizeInternalNote(note)
	if err != nil {
		return err
	}
	t := now.UTC()
	c.State, c.DecidedBy, c.DecidedAt, c.InternalNote = state, by, &t, n
	return nil
}

// Reject rejeita a Contribuição pendente com um motivo público da lista e
// uma nota interna opcional (nunca pública).
func (c *Contribution) Reject(by string, reason RejectReason, note string, now time.Time) error {
	if err := reason.Validate(); err != nil {
		return err
	}
	if err := c.decide(StateRejected, by, note, now); err != nil {
		return err
	}
	c.PublicReason = string(reason)
	return nil
}

// Accept aceita a Contribuição pendente. O efeito de cada tipo (evento
// manual, Fonte oficial) é do caso de uso, na mesma transação.
func (c *Contribution) Accept(by, note string, now time.Time) error {
	return c.decide(StateAccepted, by, note, now)
}

// CorrectSphere corrige a Esfera escolhida pelo Leitor, enquanto
// pendente. A Atualização de tramitação herda a Esfera da Proposição.
func (c *Contribution) CorrectSphere(s Sphere) error {
	if c.State != StatePending {
		return ErrNotPending
	}
	if c.Kind == KindProgressUpdate {
		return ErrSphereFixed
	}
	if err := s.Validate(); err != nil {
		return err
	}
	c.Sphere = s
	return nil
}

// LinkProposition guarda a Proposição criada a partir da Indicação
// aceita. Uma vez só.
func (c *Contribution) LinkProposition(propositionID string) error {
	if c.Kind != KindIndication || c.State != StateAccepted || c.CreatedPropositionID != "" || propositionID == "" {
		return ErrNotLinkable
	}
	c.CreatedPropositionID = propositionID
	return nil
}

// Subject é o que identifica a Contribuição numa lista da equipe: o
// domínio pedido, ou o host do link.
func (c Contribution) Subject() string {
	if c.Payload.Domain != "" {
		return c.Payload.Domain
	}
	if u, err := url.Parse(c.Payload.Link); err == nil {
		return u.Hostname()
	}
	return ""
}

// Chambers são as Casas em que a v1 grava Tramitação: as mesmas que a
// Importação conhece. Uma Casa estadual ou municipal entra aqui junto com
// a Importação dela.
var Chambers = []string{"camara", "senado"}

// ProgressEvent é o evento manual que a equipe revisou antes de aceitar
// uma Atualização de tramitação.
type ProgressEvent struct {
	Chamber     string
	At          time.Time
	Description string
	SourceURL   string
}

// NewProgressEvent valida o evento revisado: Casa (uma de Chambers), data AAAA-MM-DD (a mesma janela do formulário), texto e link
// de fonte. A data vira meio-dia de Brasília, para o dia não mudar ao
// mostrar.
func NewProgressEvent(chamber, date, description, sourceURL string, now time.Time) (ProgressEvent, error) {
	chamber = strings.TrimSpace(chamber)
	if !slices.Contains(Chambers, chamber) {
		return ProgressEvent{}, ErrInvalidChamber
	}
	d, err := parseEventDate(date, now)
	if err != nil {
		return ProgressEvent{}, err
	}
	at, _ := time.Parse(time.RFC3339, d+"T12:00:00-03:00")
	description = strings.Join(strings.Fields(description), " ")
	if description == "" || utf8.RuneCountInString(description) > MaxEventDescriptionLen {
		return ProgressEvent{}, ErrInvalidEventText
	}
	l, err := ParseLink(sourceURL)
	if err != nil {
		return ProgressEvent{}, err
	}
	return ProgressEvent{Chamber: chamber, At: at.UTC(), Description: description, SourceURL: l.URL}, nil
}
