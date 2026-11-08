package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain/structuring"
)

// StructuringState é o andamento da Estruturação de uma Versão.
type StructuringState string

const (
	// StructuringFetching: o Nelisa ainda vai buscar e ler o PDF oficial.
	StructuringFetching StructuringState = "fetching"
	// StructuringReadyToCheck: há Dispositivos; o Editor confere e corrige.
	StructuringReadyToCheck StructuringState = "ready_to_check"
	// StructuringConfirmed: o Editor confirmou; dá para comentar.
	StructuringConfirmed StructuringState = "confirmed"
	// StructuringManual: a busca ou a leitura falhou; o Editor cola o texto.
	StructuringManual StructuringState = "manual"
)

// StructuringFailure diz por que a Estruturação ficou manual.
type StructuringFailure string

const (
	FailureNone           StructuringFailure = ""
	FailureNoURL          StructuringFailure = "no_url"
	FailureHostNotAllowed StructuringFailure = "host_not_allowed"
	FailureNotFound       StructuringFailure = "not_found"
	FailureNotPDF         StructuringFailure = "not_pdf"
	FailureTooLarge       StructuringFailure = "too_large"
	FailureNoText         StructuringFailure = "no_text"
	FailureNetwork        StructuringFailure = "network"
)

// MaxSourceTextLen limita o texto de uma Versão (o PL 2338/2023, com 80
// artigos e justificação, tem uns 100 mil caracteres).
const MaxSourceTextLen = 500_000

var (
	ErrStructuringNotFound = errors.New("estruturação não encontrada")
	// ErrEmptySourceText: reprocessar exige texto.
	ErrEmptySourceText = errors.New("proposition: cole ou escreva o texto da Versão")
	// ErrSourceTextTooLong: mais que MaxSourceTextLen caracteres.
	ErrSourceTextTooLong = errors.New("proposition: texto da Versão longo demais")
	// ErrStructuringNotReady: confirmar exige Dispositivos a conferir.
	ErrStructuringNotReady = errors.New("proposition: não há Estruturação a confirmar")
	// ErrCannotRetryStructuring: só se busca de novo uma Estruturação manual
	// cujo link é de Fonte oficial com API.
	ErrCannotRetryStructuring = errors.New("proposition: não há busca a refazer")
)

// officialTextHosts são as Fontes oficiais com API cadastrada: só delas o
// Nelisa busca o PDF sozinho (CONTEXT.md, "Estruturação"; AGENTS.md, "nada
// de URL arbitrária").
var officialTextHosts = map[string]bool{
	"www.camara.leg.br":   true,
	"camara.leg.br":       true,
	"legis.senado.leg.br": true,
	"legis.senado.gov.br": true,
	"www25.senado.leg.br": true,
	"www.senado.leg.br":   true,
}

// OfficialTextFetchable diz se o Nelisa pode buscar este link sozinho:
// https e host de Fonte oficial com API.
func OfficialTextFetchable(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	return officialTextHosts[strings.ToLower(u.Hostname())]
}

// Structuring é a Estruturação de uma Versão (1:1). SourceText é o texto
// que gerou os Dispositivos: interno, nunca vai ao público.
type Structuring struct {
	VersionID   string
	State       StructuringState
	Failure     StructuringFailure
	SourceText  string
	Warnings    []structuring.Warning
	Revision    int
	UpdatedBy   string // "" quando foi o Nelisa
	UpdatedAt   time.Time
	ConfirmedBy string
	ConfirmedAt *time.Time
}

// NewStructuring começa a Estruturação de uma Versão nova: busca sozinho
// se o link é de Fonte oficial com API; senão já nasce manual.
func NewStructuring(v Version, now time.Time) Structuring {
	s := Structuring{VersionID: v.ID, State: StructuringFetching, UpdatedAt: now}
	switch {
	case strings.TrimSpace(v.OfficialTextURL) == "":
		s.State, s.Failure = StructuringManual, FailureNoURL
	case !OfficialTextFetchable(v.OfficialTextURL):
		s.State, s.Failure = StructuringManual, FailureHostNotAllowed
	}
	return s
}

// Fail deixa a Estruturação manual, com o motivo.
func (s *Structuring) Fail(f StructuringFailure, now time.Time) {
	s.State, s.Failure, s.UpdatedAt = StructuringManual, f, now
}

// Structured grava o texto e o resultado do parser: o Editor confere. by
// é "" quando foi a busca automática.
func (s *Structuring) Structured(text string, r structuring.Result, by string, now time.Time) {
	s.State, s.Failure = StructuringReadyToCheck, FailureNone
	s.SourceText, s.Warnings = text, r.Warnings
	s.Revision++
	s.UpdatedBy, s.UpdatedAt = by, now
	s.ConfirmedBy, s.ConfirmedAt = "", nil
}

// Reprocess troca o texto pelo que o Editor corrigiu ou colou e refaz os
// Dispositivos. Volta a "a conferir" mesmo se estava confirmada.
func (s *Structuring) Reprocess(text, editorID string, now time.Time) (structuring.Result, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return structuring.Result{}, ErrEmptySourceText
	}
	if len([]rune(text)) > MaxSourceTextLen {
		return structuring.Result{}, ErrSourceTextTooLong
	}
	r := structuring.Parse(text)
	s.Structured(text, r, editorID, now)
	return r, nil
}

// Confirm é o Editor dizendo que os Dispositivos estão certos: dá para
// comentar e não bloqueia mais a aprovação.
func (s *Structuring) Confirm(editorID string, now time.Time) error {
	if s.State != StructuringReadyToCheck {
		return ErrStructuringNotReady
	}
	s.State = StructuringConfirmed
	s.Revision++
	s.UpdatedBy, s.UpdatedAt = editorID, now
	t := now
	s.ConfirmedBy, s.ConfirmedAt = editorID, &t
	return nil
}

// Retry volta a buscar o PDF oficial de uma Estruturação manual (a fonte
// estava fora do ar, o PDF saiu depois). Sem sentido se o link nem é de
// Fonte oficial com API.
func (s *Structuring) Retry(v Version, editorID string, now time.Time) error {
	if s.State != StructuringManual || s.Failure == FailureNoURL || s.Failure == FailureHostNotAllowed ||
		!OfficialTextFetchable(v.OfficialTextURL) {
		return ErrCannotRetryStructuring
	}
	s.State, s.Failure = StructuringFetching, FailureNone
	s.Revision++
	s.UpdatedBy, s.UpdatedAt = editorID, now
	return nil
}

// MissingForApproval: Dispositivos ainda não buscados ou não conferidos
// impedem a aprovação. Manual sem texto não impede (a Proposição vai a
// "pronto" sem Documento comentado).
func (s Structuring) MissingForApproval() []Missing {
	switch s.State {
	case StructuringFetching:
		return []Missing{{MissingStructuring, "Aguarde a leitura do texto oficial ou cole o texto da Versão."}}
	case StructuringReadyToCheck:
		return []Missing{{MissingStructuring, "Confira e confirme a Estruturação do texto."}}
	}
	return nil
}

// Provision é um Dispositivo gravado de uma Versão, na ordem de leitura.
type Provision struct {
	VersionID string
	structuring.Provision
	Position int
	// TextHash identifica o texto (sem diferença de espaços): Comentário
	// de Dispositivo com o mesmo endereço e o mesmo hash é trazido para a
	// Versão nova.
	TextHash string
}

// ProvisionsFrom numera os Dispositivos do parser para gravar.
func ProvisionsFrom(versionID string, r structuring.Result) []Provision {
	out := make([]Provision, len(r.Provisions))
	for i, p := range r.Provisions {
		out[i] = Provision{VersionID: versionID, Provision: p, Position: i, TextHash: TextHash(p.Text)}
	}
	return out
}

// TextHash é o sha256 do texto com espaços normalizados.
func TextHash(text string) string {
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(text), " ")))
	return hex.EncodeToString(sum[:])
}
