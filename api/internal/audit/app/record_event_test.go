package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/audit/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

type fakeRepo struct {
	events []domain.Event
	err    error
}

func (f *fakeRepo) Insert(_ context.Context, e domain.Event) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, e)
	return nil
}

func TestRecordEventStampsClock(t *testing.T) {
	t0 := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{}
	uc := NewRecordEvent(repo, &clock.Fixed{T: t0})
	err := uc.Execute(context.Background(), RecordEventInput{
		ActorID: "a", Action: "user.create", TargetType: "user", TargetID: "u",
		Metadata: map[string]any{"role": "editor"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 || !repo.events[0].At.Equal(t0) || repo.events[0].Metadata["role"] != "editor" {
		t.Fatalf("events = %+v", repo.events)
	}
}

func TestRecordEventRejectsInvalidAndPropagatesFailure(t *testing.T) {
	uc := NewRecordEvent(&fakeRepo{}, &clock.Fixed{})
	if err := uc.Execute(context.Background(), RecordEventInput{Action: "x"}); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Errorf("invalid: err = %v", err)
	}
	boom := errors.New("boom")
	uc = NewRecordEvent(&fakeRepo{err: boom}, &clock.Fixed{})
	err := uc.Execute(context.Background(), RecordEventInput{Action: "a", TargetType: "t", TargetID: "1"})
	if !errors.Is(err, boom) {
		t.Errorf("failure must reach the caller so its transaction rolls back: %v", err)
	}
}
