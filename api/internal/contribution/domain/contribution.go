package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Kind é o tipo da Contribuição.
type Kind string

const (
	// KindIndication: Indicação (endereço de uma Proposição nova).
	KindIndication Kind = "indication"
	// KindProgressUpdate: Atualização de tramitação de uma Proposição
	// existente.
	KindProgressUpdate Kind = "progress_update"
	// KindSourceRequest: Pedido de fonte oficial.
	KindSourceRequest Kind = "source_request"
)

// State é o estado da Contribuição (CONTEXT.md): pendente → aceita ou
// rejeitada.
type State string

const (
	StatePending  State = "pending"
	StateAccepted State = "accepted"
	StateRejected State = "rejected"
)

// Sphere é a Esfera, com os mesmos valores do módulo proposition.
type Sphere string

const (
	SphereFederal   Sphere = "federal"
	SphereState     Sphere = "state"
	SphereMunicipal Sphere = "municipal"
)

// EventType é o tipo de evento de uma Atualização de tramitação.
type EventType string

const (
	EventVoteScheduled     EventType = "vote_scheduled"
	EventVotePostponed     EventType = "vote_postponed"
	EventApproved          EventType = "approved"
	EventRejected          EventType = "rejected"
	EventRemovedFromAgenda EventType = "removed_from_agenda"
	EventOther             EventType = "other"
)

// MaxNoteLen limita o texto livre do Leitor.
const MaxNoteLen = 1000

// MaxFutureEvent: data de evento até 1 ano no futuro (votação marcada).
const MaxFutureEvent = 366 * 24 * time.Hour

var (
	ErrContributionNotFound = errors.New("contribution: contribuição não encontrada")
	ErrInvalidKind          = errors.New("contribution: tipo de contribuição inválido")
	ErrInvalidSphere        = errors.New("contribution: esfera inválida")
	ErrInvalidEventType     = errors.New("contribution: tipo de evento inválido")
	// ErrInvalidEventDate: data que não é AAAA-MM-DD, antes de 1988 ou
	// mais de um ano no futuro.
	ErrInvalidEventDate = errors.New("contribution: data do evento inválida")
	ErrNoteTooLong      = errors.New("contribution: observação longa demais")
	// ErrNotOfficialSource: o link da Indicação não é de Fonte oficial.
	ErrNotOfficialSource = errors.New("contribution: o link não é de uma fonte oficial")
	// ErrPropositionNotFound: a Atualização é de Proposição que não existe
	// ou não é pública.
	ErrPropositionNotFound = errors.New("contribution: proposição não encontrada")
)

func (s Sphere) Validate() error {
	switch s {
	case SphereFederal, SphereState, SphereMunicipal:
		return nil
	}
	return ErrInvalidSphere
}

func (e EventType) Validate() error {
	switch e {
	case EventVoteScheduled, EventVotePostponed, EventApproved, EventRejected, EventRemovedFromAgenda, EventOther:
		return nil
	}
	return ErrInvalidEventType
}

// Payload é o que o Leitor mandou, conforme o tipo. Os campos que não são
// do tipo ficam vazios.
type Payload struct {
	// Indicação: Link da Proposição. Atualização: Link da fonte. Pedido de
	// fonte oficial: Link da evidência.
	Link string `json:"link"`
	// Atualização de tramitação.
	EventType EventType `json:"event_type,omitempty"`
	EventDate string    `json:"event_date,omitempty"`
	// Pedido de fonte oficial.
	Domain     string `json:"domain,omitempty"`
	SourceName string `json:"source_name,omitempty"`
	Note       string `json:"note,omitempty"`
}

// Contribution é uma Contribuição. Nunca altera a Proposição enquanto
// pendente.
type Contribution struct {
	ID    string
	Kind  Kind
	State State
	// TrackingHash é o hash do código de acompanhamento.
	TrackingHash  string
	PropositionID string // só na Atualização de tramitação
	Sphere        Sphere
	Payload       Payload
	SubmittedAt   time.Time
	DecidedBy     string
	DecidedAt     *time.Time
	PublicReason  string
	InternalNote  string
	// CreatedPropositionID: a Proposição criada a partir da Indicação.
	CreatedPropositionID string
}

// Submission é o formulário, ainda não validado.
type Submission struct {
	Kind       Kind
	Link       string
	Sphere     Sphere
	EventType  EventType
	EventDate  string
	Domain     string
	SourceName string
	Note       string
}

// PropositionRef é a Proposição pública de uma Atualização de tramitação.
type PropositionRef struct {
	ID     string
	Sphere Sphere
}

// Context é o que a validação precisa saber de fora.
type Context struct {
	// Proposition: a Proposição da Atualização (nil se não foi achada).
	Proposition *PropositionRef
	// Registered: domínios cadastrados como Fonte oficial.
	Registered []string
	Now        time.Time
}

// NewContribution valida o formulário e monta a Contribuição pendente.
func NewContribution(id string, code TrackingCode, s Submission, c Context) (Contribution, error) {
	note, err := normalizeNote(s.Note)
	if err != nil {
		return Contribution{}, err
	}
	out := Contribution{
		ID: id, Kind: s.Kind, State: StatePending, TrackingHash: code.Hash(), SubmittedAt: c.Now,
		Payload: Payload{Note: note},
	}
	switch s.Kind {
	case KindIndication:
		l, err := ParseLink(s.Link)
		if err != nil {
			return Contribution{}, err
		}
		if !IsOfficial(l.Host, c.Registered) {
			return Contribution{}, ErrNotOfficialSource
		}
		if err := s.Sphere.Validate(); err != nil {
			return Contribution{}, err
		}
		out.Sphere, out.Payload.Link = s.Sphere, l.URL

	case KindProgressUpdate:
		if c.Proposition == nil {
			return Contribution{}, ErrPropositionNotFound
		}
		l, err := ParseLink(s.Link)
		if err != nil {
			return Contribution{}, err
		}
		if err := s.EventType.Validate(); err != nil {
			return Contribution{}, err
		}
		date, err := parseEventDate(s.EventDate, c.Now)
		if err != nil {
			return Contribution{}, err
		}
		out.PropositionID, out.Sphere = c.Proposition.ID, c.Proposition.Sphere
		out.Payload.Link, out.Payload.EventType, out.Payload.EventDate = l.URL, s.EventType, date

	case KindSourceRequest:
		d, err := NormalizeDomain(s.Domain)
		if err != nil {
			return Contribution{}, err
		}
		if IsOfficial(d, c.Registered) {
			return Contribution{}, ErrOfficialSourceExists
		}
		l, err := ParseLink(s.Link)
		if err != nil {
			return Contribution{}, err
		}
		name, err := normalizeName(s.SourceName)
		if err != nil {
			return Contribution{}, err
		}
		if err := s.Sphere.Validate(); err != nil {
			return Contribution{}, err
		}
		out.Sphere = s.Sphere
		out.Payload.Domain, out.Payload.Link, out.Payload.SourceName = d, l.URL, name

	default:
		return Contribution{}, ErrInvalidKind
	}
	return out, nil
}

func normalizeNote(s string) (string, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if utf8.RuneCountInString(s) > MaxNoteLen {
		return "", ErrNoteTooLong
	}
	return s, nil
}

// minEventDate: nada antes da Constituição de 1988.
var minEventDate = time.Date(1988, 10, 5, 0, 0, 0, 0, time.UTC)

func parseEventDate(s string, now time.Time) (string, error) {
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	if err != nil || d.Before(minEventDate) || d.After(now.Add(MaxFutureEvent)) {
		return "", ErrInvalidEventDate
	}
	return d.Format(time.DateOnly), nil
}
