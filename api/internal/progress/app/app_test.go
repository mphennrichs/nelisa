package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/progress/app"
	"github.com/mphennrichs/nelisa/api/internal/progress/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/progress/domain"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func TestRecordEntriesIsIdempotentAndTimelineIsMerged(t *testing.T) {
	repo := fakes.NewEntries()
	rec := app.NewRecordEntries(repo, &clock.Fixed{T: t0})
	ctx := context.Background()
	camara := app.RecordEntriesInput{PropositionID: "p1", Chamber: "camara", Entries: []app.EntryInput{
		{ExternalKey: "1-100", At: t0, Description: "Apresentação"},
		{ExternalKey: "2-200", At: t0.Add(48 * time.Hour), Description: "Aprovada no Plenário"},
	}}
	for i := range 2 {
		res, err := rec.Execute(ctx, camara)
		if err != nil {
			t.Fatal(err)
		}
		want := 2
		if i == 1 {
			want = 0
		}
		if res.Inserted != want || !res.LatestAt.Equal(t0.Add(48*time.Hour)) {
			t.Errorf("run %d: %+v", i, res)
		}
	}
	if repo.Len() != 2 {
		t.Fatalf("duplicated entries: %d", repo.Len())
	}
	if _, err := rec.Execute(ctx, app.RecordEntriesInput{PropositionID: "p1", Chamber: "senado", Entries: []app.EntryInput{
		{ExternalKey: "77", At: t0.Add(24 * time.Hour), Description: "Recebida no Senado"},
		{ExternalKey: "77", At: t0.Add(24 * time.Hour), Description: "Recebida no Senado (corrigido)"},
	}}); err != nil {
		t.Fatal(err)
	}
	tl, err := app.NewTimeline(repo).Execute(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range tl {
		got = append(got, e.Chamber+":"+e.ExternalKey)
	}
	if strings.Join(got, ",") != "camara:1-100,senado:77,camara:2-200" {
		t.Errorf("timeline order = %v", got)
	}
	if tl[1].Description != "Recebida no Senado (corrigido)" {
		t.Errorf("duplicate key in one response: last wins, got %q", tl[1].Description)
	}
}

func TestRecordEntriesRejectsHostileInput(t *testing.T) {
	rec := app.NewRecordEntries(fakes.NewEntries(), &clock.Fixed{T: t0})
	ctx := context.Background()
	if _, err := rec.Execute(ctx, app.RecordEntriesInput{PropositionID: "p", Chamber: "camara", Entries: []app.EntryInput{{ExternalKey: "", At: t0}}}); !errors.Is(err, domain.ErrInvalidEntry) {
		t.Errorf("empty key: %v", err)
	}
	if _, err := rec.Execute(ctx, app.RecordEntriesInput{PropositionID: "p", Chamber: "camara", Entries: []app.EntryInput{{ExternalKey: "1"}}}); !errors.Is(err, domain.ErrInvalidEntry) {
		t.Errorf("zero date: %v", err)
	}
	many := make([]app.EntryInput, domain.MaxEntriesPerSync+1)
	if _, err := rec.Execute(ctx, app.RecordEntriesInput{PropositionID: "p", Chamber: "camara", Entries: many}); !errors.Is(err, app.ErrTooManyEntries) {
		t.Errorf("too many: %v", err)
	}
	e, err := domain.NewEntry("p", "camara", "k", t0, "PLEN", strings.Repeat("x", 9_000), "", "")
	if err != nil || len(e.Description) != 5_000 {
		t.Errorf("long description not truncated: %v %d", err, len(e.Description))
	}
}
