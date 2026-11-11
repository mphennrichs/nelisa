package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/store"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
)

var t0 = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

func at(d time.Duration) *time.Time { t := t0.Add(d); return &t }

func TestRadarRoundTrip(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	s := store.NewRadar(pool)
	const ana = "00000000-0000-4000-8000-0000000000e1"
	pl := app.RadarHit{Chamber: app.ChamberCamara, ExternalID: "100", Kind: "PL", Number: 1, Year: 2026, Summary: "Ementa", SourceURL: "https://camara.leg.br/1", PresentedAt: at(-24 * time.Hour)}
	pec := app.RadarHit{Chamber: app.ChamberSenado, ExternalID: "200", Kind: "PEC", Number: 2, Year: 2026, AgendaAt: at(48 * time.Hour)}
	if err := s.Upsert(ctx, []app.RadarHit{pl, pec}, t0); err != nil {
		t.Fatal(err)
	}
	vis := func() []app.RadarItem {
		t.Helper()
		v, err := s.Visible(ctx, t0.Add(-24*time.Hour), t0.Add(-30*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := vis(); len(v) != 2 || v[0].ExternalID != "200" || v[1].Summary != "Ementa" {
		t.Fatalf("visíveis = %+v", v)
	}

	// O PL entra em pauta: a apresentação fica, a pauta chega, a ementa
	// vazia não apaga a guardada.
	pl2 := pl
	pl2.PresentedAt, pl2.AgendaAt, pl2.Summary = nil, at(72*time.Hour), ""
	if err := s.Upsert(ctx, []app.RadarHit{pl2}, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Dispensado some; volta só com pauta posterior.
	it, err := s.Dismiss(ctx, app.ChamberCamara, "100", ana, t0.Add(2*time.Hour))
	if err != nil || it.DismissedAt == nil || it.PresentedAt == nil || it.AgendaAt == nil || it.Summary != "Ementa" {
		t.Fatalf("dismiss = %+v, %v", it, err)
	}
	if v := vis(); len(v) != 1 || v[0].ExternalID != "200" {
		t.Fatalf("depois de dispensar = %+v", v)
	}
	if err := s.Upsert(ctx, []app.RadarHit{pl2}, t0.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if v := vis(); len(v) != 1 {
		t.Fatalf("a mesma pauta não traz de volta: %+v", v)
	}
	pl3 := pl2
	pl3.AgendaAt = at(96 * time.Hour)
	if err := s.Upsert(ctx, []app.RadarHit{pl3}, t0.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if v := vis(); len(v) != 2 {
		t.Fatalf("pauta nova traz de volta: %+v", v)
	}

	if _, err := s.Dismiss(ctx, app.ChamberCamara, "999", ana, t0); !errors.Is(err, app.ErrRadarItemNotFound) {
		t.Errorf("dispensar inexistente: %v", err)
	}
	n, err := s.Purge(ctx, t0.Add(2*time.Hour))
	if err != nil || n != 1 {
		t.Errorf("purge = %d, %v", n, err)
	}
}
