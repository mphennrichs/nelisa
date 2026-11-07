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

// ErrInvalidEntry: evento sem Proposição, Casa, chave ou data.
var ErrInvalidEntry = errors.New("progress: evento de tramitação inválido")

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
}

// NewEntry valida e limpa um evento vindo da fonte.
func NewEntry(propositionID, chamber, externalKey string, at time.Time, body, description, situation, dispatch string) (Entry, error) {
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
