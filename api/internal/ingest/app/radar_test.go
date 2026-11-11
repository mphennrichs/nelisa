package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

var now = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

func tp(d time.Duration) *time.Time { t := now.Add(d); return &t }

func hit(c app.ChamberCode, id, kind string, agenda, presented *time.Time) app.RadarHit {
	return app.RadarHit{Chamber: c, ExternalID: id, Kind: kind, Number: 1, Year: 2026, AgendaAt: agenda, PresentedAt: presented, SourceURL: "https://x.leg.br/" + id}
}

func TestRadarScanListDismiss(t *testing.T) {
	e := newEnv()
	repo := fakes.NewRadar()
	clk := &clock.Fixed{T: now}
	// A PEC 221/2019 (2192459) já está no Nelisa: não aparece no Radar.
	if _, err := e.imp().Execute(editor, pec); err != nil {
		t.Fatal(err)
	}
	e.camara.AgendaHits = []app.RadarHit{
		hit(app.ChamberCamara, "10", "PL", tp(48*time.Hour), nil),
		hit(app.ChamberCamara, "10", "PL", tp(24*time.Hour), nil), // mesma, outra sessão
		hit(app.ChamberCamara, "2192459", "PEC", tp(24*time.Hour), nil),
		hit(app.ChamberCamara, "11", "REQ", tp(24*time.Hour), nil), // Tipo fora do Radar
	}
	e.camara.PresentedHits = []app.RadarHit{hit(app.ChamberCamara, "12", "PLP", nil, tp(-24*time.Hour))}
	e.senado.RadarErr = fakes.Unavailable()

	res, err := app.NewScanRadar(e.sources, repo, clk).Execute(context.Background())
	if !errors.Is(err, app.ErrUnavailable) {
		t.Errorf("Senado fora do ar deve voltar como erro: %v", err)
	}
	if res.Found != 3 || repo.Len() != 3 {
		t.Fatalf("achados = %+v, guardados %d", res, repo.Len())
	}

	list := app.NewListRadar(repo, e.w.Catalog(), clk)
	if _, err := list.Execute(context.Background()); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("anônimo: %v", err)
	}
	view, err := list.Execute(editor)
	if err != nil {
		t.Fatal(err)
	}
	items := view.Items
	if view.LastSeen == nil || !view.LastSeen.Equal(now) {
		t.Errorf("última rodada = %v", view.LastSeen)
	}
	if len(items) != 2 || items[0].ExternalID != "10" || !items[0].AgendaAt.Equal(*tp(48 * time.Hour)) || items[1].ExternalID != "12" {
		t.Fatalf("radar = %+v", items)
	}

	dismiss := app.NewDismissRadarItem(repo, e.w.Tx(), e.w.Audit(), clk)
	if err := dismiss.Execute(as("x", auth.Role("reader")), app.ChamberCamara, "12"); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("leitor: %v", err)
	}
	if err := dismiss.Execute(editor, app.ChamberCamara, "12"); err != nil {
		t.Fatal(err)
	}
	if err := dismiss.Execute(editor, app.ChamberCamara, "999"); !errors.Is(err, app.ErrRadarItemNotFound) {
		t.Errorf("inexistente: %v", err)
	}
	evs := e.w.Events()
	if last := evs[len(evs)-1]; last.Action != app.ActionRadarDismiss || last.TargetID != "camara:12" || !last.InTx {
		t.Errorf("auditoria = %+v", last)
	}
	view, _ = list.Execute(editor)
	items = view.Items
	if len(items) != 1 || items[0].ExternalID != "10" {
		t.Fatalf("depois de dispensar = %+v", items)
	}

	// O PLP dispensado entra em pauta: volta.
	e.senado.RadarErr = nil
	e.camara.PresentedHits = nil
	e.camara.AgendaHits = []app.RadarHit{hit(app.ChamberCamara, "12", "PLP", tp(72*time.Hour), nil)}
	if _, err := app.NewScanRadar(e.sources, repo, clk).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	view, _ = list.Execute(editor)
	items = view.Items
	if len(items) != 2 || items[1].ExternalID != "12" {
		t.Fatalf("pauta nova traz de volta: %+v", items)
	}
}

func TestRequestRadarScan(t *testing.T) {
	s := &fakes.RadarScheduler{}
	uc := app.NewRequestRadarScan(s)
	if err := uc.Execute(context.Background()); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("anônimo: %v", err)
	}
	if err := uc.Execute(editor); err != nil || s.Calls != 1 {
		t.Errorf("pedido = %v, %d", err, s.Calls)
	}
}
