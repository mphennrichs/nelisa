// Package app tem os casos de uso da Tramitação e a porta de que precisam.
// São operações de sistema: quem chama (Importação, Sincronização, a
// página da Proposição) já autorizou e já aplicou a regra de visibilidade,
// pelas portas ligadas no main.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/progress/domain"
)

// ErrTooManyEntries: a fonte mandou mais eventos do que é plausível.
var ErrTooManyEntries = errors.New("progress: eventos demais numa sincronização")

// EntryRepo persiste a Tramitação. Usa a transação do contexto quando há.
type EntryRepo interface {
	// Upsert grava os eventos; um evento que já existe (mesma Proposição,
	// Casa e chave) é atualizado, nunca duplicado. Devolve quantos eram
	// novos.
	Upsert(ctx context.Context, es []domain.Entry, now time.Time) (inserted int, err error)
	// ListByProposition devolve a linha do tempo, já ordenada.
	ListByProposition(ctx context.Context, propositionID string) ([]domain.Entry, error)
	// InsertManual grava um evento manual; ErrManualEntryExists se a
	// chave já existe.
	InsertManual(ctx context.Context, e domain.Entry, now time.Time) error
	// DeleteManual apaga o evento manual com a chave; false se não há.
	DeleteManual(ctx context.Context, externalKey string) (bool, error)
}

// EntryInput é um evento como a fonte o descreve.
type EntryInput struct {
	ExternalKey string
	At          time.Time
	Body        string
	Description string
	Situation   string
	Dispatch    string
}

type RecordEntriesInput struct {
	PropositionID string
	Chamber       string
	Entries       []EntryInput
}

type RecordEntriesResult struct {
	Inserted int
	// LatestAt é a data do evento mais recente recebido (nil se nenhum).
	LatestAt *time.Time
}

// RecordEntries aplica a Tramitação de uma Casa. Rodar duas vezes com os
// mesmos eventos não duplica nada.
type RecordEntries struct {
	repo  EntryRepo
	clock clock.Clock
}

func NewRecordEntries(repo EntryRepo, clk clock.Clock) *RecordEntries {
	return &RecordEntries{repo: repo, clock: clk}
}

func (uc *RecordEntries) Execute(ctx context.Context, in RecordEntriesInput) (RecordEntriesResult, error) {
	if len(in.Entries) > domain.MaxEntriesPerSync {
		return RecordEntriesResult{}, ErrTooManyEntries
	}
	var res RecordEntriesResult
	seen := make(map[string]int, len(in.Entries))
	es := make([]domain.Entry, 0, len(in.Entries))
	for _, raw := range in.Entries {
		e, err := domain.NewEntry(in.PropositionID, in.Chamber, raw.ExternalKey, raw.At,
			raw.Body, raw.Description, raw.Situation, raw.Dispatch)
		if err != nil {
			return RecordEntriesResult{}, fmt.Errorf("evento %q: %w", raw.ExternalKey, err)
		}
		// A mesma chave duas vezes na resposta: vale a última.
		if i, ok := seen[e.ExternalKey]; ok {
			es[i] = e
		} else {
			seen[e.ExternalKey] = len(es)
			es = append(es, e)
		}
		if res.LatestAt == nil || e.At.After(*res.LatestAt) {
			t := e.At
			res.LatestAt = &t
		}
	}
	if len(es) == 0 {
		return res, nil
	}
	n, err := uc.repo.Upsert(ctx, es, uc.clock.Now())
	if err != nil {
		return RecordEntriesResult{}, err
	}
	res.Inserted = n
	return res, nil
}

// Timeline devolve a linha do tempo única de uma Proposição, de todas as
// Casas, ordenada por data.
type Timeline struct{ repo EntryRepo }

func NewTimeline(repo EntryRepo) *Timeline { return &Timeline{repo: repo} }

func (uc *Timeline) Execute(ctx context.Context, propositionID string) ([]domain.Entry, error) {
	es, err := uc.repo.ListByProposition(ctx, propositionID)
	if err != nil {
		return nil, err
	}
	domain.SortTimeline(es)
	return es, nil
}

// ManualEntryInput é o evento manual de uma Atualização de tramitação
// aceita, já revisado pela equipe.
type ManualEntryInput struct {
	PropositionID  string
	Chamber        string
	ContributionID string
	At             time.Time
	Description    string
	SourceURL      string
}

// ManualEntries grava e apaga eventos manuais. Operação de sistema: o
// módulo contribution autoriza e audita, na transação do contexto.
type ManualEntries struct {
	repo  EntryRepo
	clock clock.Clock
}

func NewManualEntries(repo EntryRepo, clk clock.Clock) *ManualEntries {
	return &ManualEntries{repo: repo, clock: clk}
}

// Record grava o evento; ErrManualEntryExists se a Contribuição já gravou
// o dela.
func (uc *ManualEntries) Record(ctx context.Context, in ManualEntryInput) error {
	e, err := domain.NewManualEntry(in.PropositionID, in.Chamber, in.ContributionID, in.At, in.Description, in.SourceURL)
	if err != nil {
		return err
	}
	return uc.repo.InsertManual(ctx, e, uc.clock.Now())
}

// Delete apaga o evento manual da Contribuição; false se não há.
func (uc *ManualEntries) Delete(ctx context.Context, contributionID string) (bool, error) {
	if contributionID == "" {
		return false, nil
	}
	return uc.repo.DeleteManual(ctx, domain.ManualKey(contributionID))
}
