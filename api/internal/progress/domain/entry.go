// Package domain tem a Tramitação: os eventos do percurso legislativo de
// uma Proposição, vindos de todas as Casas por onde ela passou, numa linha
// do tempo única (ADR 0006). Só Go padrão.
package domain

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrInvalidEntry: evento sem Proposição, Casa, chave ou data, ou
	// evento manual sem link de fonte.
	ErrInvalidEntry = errors.New("progress: evento de tramitação inválido")
	// ErrManualEntryExists: a Contribuição já gravou o seu evento manual.
	ErrManualEntryExists = errors.New("progress: evento manual já gravado")
)

// ManualKeyPrefix marca a chave de um evento manual: contribution:<id>. A
// fonte nunca usa essa chave, então a Sincronização não sobrescreve o
// evento manual.
const ManualKeyPrefix = "contribution:"

// MaxSourceURLLen limita o link de fonte de um evento manual.
const MaxSourceURLLen = 2000

// Limites: a fonte é tratada como hostil.
const (
	maxKeyLen         = 128
	maxBodyLen        = 200
	maxDescriptionLen = 5_000
	maxSituationLen   = 1_000
	maxDispatchLen    = 20_000
	// MaxEntriesPerSync limita quantos eventos uma Casa pode trazer de uma
	// vez; acima disso a resposta é tratada como inválida.
	MaxEntriesPerSync = 5_000
)

// Entry é um evento de Tramitação numa Casa.
type Entry struct {
	PropositionID string
	Chamber       string
	// ExternalKey identifica o evento na fonte; com a Casa e a Proposição,
	// torna a Sincronização idempotente.
	ExternalKey string
	At          time.Time
	Body        string
	Description string
	Situation   string
	Dispatch    string
	// Manual: evento de uma Atualização de tramitação aceita, não da
	// fonte. Sempre com SourceURL.
	Manual    bool
	SourceURL string
}

// ContributionID é o id da Contribuição de um evento manual ("" se não
// for manual).
func (e Entry) ContributionID() string {
	if !e.Manual {
		return ""
	}
	return strings.TrimPrefix(e.ExternalKey, ManualKeyPrefix)
}

// ManualKey é a chave do evento manual de uma Contribuição.
func ManualKey(contributionID string) string { return ManualKeyPrefix + contributionID }

// NewManualEntry monta o evento manual de uma Atualização de tramitação
// aceita. O texto já foi revisado pela equipe; o link já foi validado pelo
// módulo contribution (aqui só se exige que exista e caiba).
func NewManualEntry(propositionID, chamber, contributionID string, at time.Time, description, sourceURL string) (Entry, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if strings.TrimSpace(contributionID) == "" || sourceURL == "" || len(sourceURL) > MaxSourceURLLen ||
		strings.TrimSpace(description) == "" {
		return Entry{}, ErrInvalidEntry
	}
	e, err := newEntry(propositionID, chamber, ManualKey(strings.TrimSpace(contributionID)), at, "", description, "", "")
	if err != nil {
		return Entry{}, err
	}
	e.Manual, e.SourceURL = true, sourceURL
	return e, nil
}

// NewEntry valida e limpa um evento vindo da fonte. A chave de evento
// manual é recusada: a fonte não pode se passar por uma Contribuição.
func NewEntry(propositionID, chamber, externalKey string, at time.Time, body, description, situation, dispatch string) (Entry, error) {
	if strings.HasPrefix(strings.TrimSpace(externalKey), ManualKeyPrefix) {
		return Entry{}, ErrInvalidEntry
	}
	return newEntry(propositionID, chamber, externalKey, at, body, description, situation, dispatch)
}

func newEntry(propositionID, chamber, externalKey string, at time.Time, body, description, situation, dispatch string) (Entry, error) {
	externalKey = strings.TrimSpace(externalKey)
	chamber = strings.TrimSpace(chamber)
	if propositionID == "" || chamber == "" || externalKey == "" || len(externalKey) > maxKeyLen || at.IsZero() {
		return Entry{}, ErrInvalidEntry
	}
	return Entry{
		PropositionID: propositionID, Chamber: chamber, ExternalKey: externalKey, At: at.UTC(),
		Body:        truncate(body, maxBodyLen),
		Description: truncate(description, maxDescriptionLen),
		Situation:   truncate(situation, maxSituationLen),
		Dispatch:    truncate(dispatch, maxDispatchLen),
	}, nil
}

// SortTimeline ordena a linha do tempo única: por data, depois Casa e
// chave, para a ordem ser estável entre eventos do mesmo instante.
func SortTimeline(es []Entry) {
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		if a.Chamber != b.Chamber {
			return a.Chamber < b.Chamber
		}
		return a.ExternalKey < b.ExternalKey
	})
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
