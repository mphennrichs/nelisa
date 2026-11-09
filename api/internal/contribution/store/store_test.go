package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/contribution/store"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
)

var t0 = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

const (
	c1  = "00000000-0000-4000-8000-0000000000c1"
	c2  = "00000000-0000-4000-8000-0000000000c2"
	p1  = "00000000-0000-4000-8000-000000000001"
	s1  = "00000000-0000-4000-8000-0000000000a1"
	ana = "00000000-0000-4000-8000-0000000000e1"
)

func TestContributionsRoundTrip(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	s := store.NewContributions(pool)

	code := domain.TrackingCode("ABCDEFGHJKMN")
	in := domain.Contribution{
		ID: c1, Kind: domain.KindProgressUpdate, State: domain.StatePending, TrackingHash: code.Hash(),
		PropositionID: p1, Sphere: domain.SphereFederal, SubmittedAt: t0,
		Payload: domain.Payload{Link: "https://g1.globo.com/x", EventType: domain.EventVotePostponed, EventDate: "2026-11-20", Note: "adiada"},
	}
	if err := s.Insert(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.FindByTrackingHash(ctx, code.Hash())
	if err != nil || got.ID != c1 || got.PropositionID != p1 || got.Payload != in.Payload || !got.SubmittedAt.Equal(t0) || got.DecidedAt != nil {
		t.Fatalf("find = %+v, %v", got, err)
	}
	if _, err := s.FindByTrackingHash(ctx, domain.TrackingCode("000000000000").Hash()); !errors.Is(err, domain.ErrContributionNotFound) {
		t.Errorf("find inexistente: %v", err)
	}

	dup := in
	dup.ID = c2
	if err := s.Insert(ctx, dup); !errors.Is(err, app.ErrTrackingCodeTaken) {
		t.Errorf("código repetido: %v", err)
	}
	// O banco recusa Atualização sem Proposição.
	bad := in
	bad.ID, bad.TrackingHash, bad.PropositionID = c2, domain.TrackingCode("111111111111").Hash(), ""
	if err := s.Insert(ctx, bad); err == nil {
		t.Error("atualização sem proposição deveria falhar")
	}

	n, err := s.CountPending(ctx)
	if err != nil || n != 1 {
		t.Errorf("pending = %d, %v", n, err)
	}
}

func TestOfficialSourcesRoundTrip(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	s := store.NewOfficialSources(pool)
	o := domain.OfficialSource{ID: s1, Domain: "cmsp.org", Name: "Câmara Municipal de SP", Sphere: domain.SphereMunicipal, CreatedBy: ana, CreatedAt: t0}
	if err := s.Insert(ctx, o); err != nil {
		t.Fatal(err)
	}
	dup := o
	dup.ID = c1
	if err := s.Insert(ctx, dup); !errors.Is(err, domain.ErrOfficialSourceExists) {
		t.Errorf("domínio repetido: %v", err)
	}
	list, err := s.List(ctx)
	if err != nil || len(list) != 1 || list[0] != o {
		t.Fatalf("list = %+v, %v", list, err)
	}
	ds, err := s.Domains(ctx)
	if err != nil || len(ds) != 1 || ds[0] != "cmsp.org" {
		t.Errorf("domains = %v, %v", ds, err)
	}
	got, err := s.Delete(ctx, s1)
	if err != nil || got != o {
		t.Fatalf("delete = %+v, %v", got, err)
	}
	for _, id := range []string{s1, "não-é-uuid"} {
		if _, err := s.Delete(ctx, id); !errors.Is(err, domain.ErrOfficialSourceNotFound) {
			t.Errorf("delete %q: %v", id, err)
		}
	}
}
