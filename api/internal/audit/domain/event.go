// Package domain tem o evento de Auditoria: quem fez o quê, em quê e
// quando. Eventos só são acrescentados; nunca mudam.
package domain

import (
	"errors"
	"strings"
	"time"
)

// ErrInvalidEvent: falta ação ou alvo. Um evento sem eles não prova nada.
var ErrInvalidEvent = errors.New("audit: evento sem ação ou alvo")

// Event é um registro de Auditoria.
type Event struct {
	// ActorID é o usuário da equipe que agiu; vazio quando é o sistema.
	ActorID string
	// Action no formato "<alvo>.<verbo>", ex.: "user.create".
	Action     string
	TargetType string
	TargetID   string
	At         time.Time
	// Metadata são detalhes da ação (ex.: role antiga e nova). Nunca senha,
	// token ou outro segredo.
	Metadata map[string]any
}

// NewEvent valida e monta um evento.
func NewEvent(actorID, action, targetType, targetID string, at time.Time, metadata map[string]any) (Event, error) {
	if strings.TrimSpace(action) == "" || strings.TrimSpace(targetType) == "" || strings.TrimSpace(targetID) == "" {
		return Event{}, ErrInvalidEvent
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	return Event{
		ActorID: actorID, Action: action, TargetType: targetType, TargetID: targetID,
		At: at, Metadata: metadata,
	}, nil
}
