package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrLinkNotFound = errors.New("proposition: vínculo de fonte não encontrado")
	// ErrLinkNotSuggested: só uma sugestão pode ser confirmada ou descartada.
	ErrLinkNotSuggested = errors.New("proposition: o vínculo não é uma sugestão")
	// ErrLinkTaken: o registro dessa Casa já está confirmado em outra
	// Proposição.
	ErrLinkTaken = errors.New("proposition: registro já vinculado a outra proposição")
	// ErrChamberAlreadyLinked: a Proposição já tem um Vínculo confirmado
	// nessa Casa.
	ErrChamberAlreadyLinked = errors.New("proposition: a proposição já tem vínculo confirmado nessa casa")
	// ErrLinkExists: o mesmo registro já está ligado (sugerido ou
	// confirmado) a esta Proposição.
	ErrLinkExists      = errors.New("proposition: vínculo já existe")
	ErrInvalidExternal = errors.New("proposition: id externo inválido")
)

// LinkState: o sistema sugere, só o Editor confirma (ADR 0006).
type LinkState string

const (
	LinkSuggested LinkState = "suggested"
	LinkConfirmed LinkState = "confirmed"
)

const maxExternalIDLen = 64

// SourceLink é o Vínculo de fonte: a Proposição numa Casa com API.
type SourceLink struct {
	ID            string
	PropositionID string
	Chamber       Chamber
	ExternalID    string
	State         LinkState
	// SourceName é como a Casa chama a Proposição; SourceURL, a página
	// pública dela na Casa.
	SourceName    string
	SourceURL     string
	ConfirmedBy   string
	ConfirmedAt   *time.Time
	LastSyncedAt  *time.Time
	LastSyncError string
	CreatedAt     time.Time
}

func newLink(id, propositionID string, chamber Chamber, externalID, sourceName, sourceURL string, now time.Time) (SourceLink, error) {
	if _, err := ParseChamber(string(chamber)); err != nil {
		return SourceLink{}, err
	}
	externalID = strings.TrimSpace(externalID)
	if externalID == "" || len(externalID) > maxExternalIDLen || strings.ContainsAny(externalID, " /?#\t\r\n") {
		return SourceLink{}, ErrInvalidExternal
	}
	sourceURL = strings.TrimSpace(sourceURL)
	if len(sourceURL) > maxURLLen || !IsHTTPURL(sourceURL) {
		sourceURL = ""
	}
	return SourceLink{
		ID: id, PropositionID: propositionID, Chamber: chamber, ExternalID: externalID,
		SourceName: truncate(strings.TrimSpace(sourceName), 100), SourceURL: sourceURL, CreatedAt: now,
	}, nil
}

// NewSuggestedLink é um candidato achado pelo sistema. Não vale nada até um
// Editor confirmar: não sincroniza nem aparece ao Leitor.
func NewSuggestedLink(id, propositionID string, chamber Chamber, externalID, sourceName, sourceURL string, now time.Time) (SourceLink, error) {
	l, err := newLink(id, propositionID, chamber, externalID, sourceName, sourceURL, now)
	l.State = LinkSuggested
	return l, err
}

// NewConfirmedLink é o vínculo que o Editor escolheu ao importar: escolher
// o registro na busca já é a confirmação.
func NewConfirmedLink(id, propositionID string, chamber Chamber, externalID, sourceName, sourceURL, editorID string, now time.Time) (SourceLink, error) {
	l, err := newLink(id, propositionID, chamber, externalID, sourceName, sourceURL, now)
	if err != nil {
		return SourceLink{}, err
	}
	l.State, l.ConfirmedBy, l.ConfirmedAt = LinkConfirmed, editorID, &now
	return l, nil
}

// Confirm é o Editor aceitando a sugestão.
func (l *SourceLink) Confirm(editorID string, now time.Time) error {
	if l.State != LinkSuggested {
		return ErrLinkNotSuggested
	}
	l.State, l.ConfirmedBy, l.ConfirmedAt = LinkConfirmed, editorID, &now
	return nil
}

// CanReject: só sugestão se descarta. Um vínculo confirmado errado é outra
// conversa (desvincular), fora da v1.
func (l SourceLink) CanReject() error {
	if l.State != LinkSuggested {
		return ErrLinkNotSuggested
	}
	return nil
}

// UpdateSource troca nome e link na Casa pelos atuais da fonte; valores
// vazios ou inválidos mantêm os anteriores.
func (l *SourceLink) UpdateSource(name, url string) {
	if name = truncate(strings.TrimSpace(name), 100); name != "" {
		l.SourceName = name
	}
	if url = strings.TrimSpace(url); url != "" && len(url) <= maxURLLen && IsHTTPURL(url) {
		l.SourceURL = url
	}
}

// maxSyncErrorLen limita a mensagem guardada: é só para a equipe saber que
// a última tentativa falhou.
const maxSyncErrorLen = 500

// SyncSucceeded registra uma Sincronização bem-sucedida.
func (l *SourceLink) SyncSucceeded(at time.Time) {
	l.LastSyncedAt, l.LastSyncError = &at, ""
}

// SyncFailed registra a falha sem apagar quando foi o último sucesso.
func (l *SourceLink) SyncFailed(msg string) {
	msg = truncate(strings.TrimSpace(msg), maxSyncErrorLen)
	if msg == "" {
		msg = "falha desconhecida"
	}
	l.LastSyncError = msg
}
