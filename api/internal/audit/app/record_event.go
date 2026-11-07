// Package app tem os casos de uso da Auditoria.
package app

import (
	"context"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/audit/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

// EventRepo grava eventos. A implementação entra na transação que estiver
// no contexto: é o que garante que mudança e evento são gravados juntos.
type EventRepo interface {
	Insert(ctx context.Context, e domain.Event) error
}

// RecordEventInput é o que os outros módulos mandam registrar.
type RecordEventInput struct {
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

// RecordEvent registra um evento de Auditoria. Os outros módulos o chamam
// (por uma porta própria, ligada no main) de dentro da transação da ação
// auditada; se falhar, a ação inteira desfaz.
type RecordEvent struct {
	repo  EventRepo
	clock clock.Clock
}

func NewRecordEvent(repo EventRepo, clk clock.Clock) *RecordEvent {
	return &RecordEvent{repo: repo, clock: clk}
}

func (uc *RecordEvent) Execute(ctx context.Context, in RecordEventInput) error {
	e, err := domain.NewEvent(in.ActorID, in.Action, in.TargetType, in.TargetID, uc.clock.Now(), in.Metadata)
	if err != nil {
		return err
	}
	if err := uc.repo.Insert(ctx, e); err != nil {
		return fmt.Errorf("gravando evento de auditoria: %w", err)
	}
	return nil
}
