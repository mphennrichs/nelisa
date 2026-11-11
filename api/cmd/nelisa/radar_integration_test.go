package main

import (
	"testing"
	"time"

	ingestapp "github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
)

// TestRadarEndToEnd: a rodada grava no Postgres o que as fontes acham; o
// que já foi importado não aparece; o resto aparece com a pauta primeiro.
func TestRadarEndToEnd(t *testing.T) {
	pool, _ := testdb.New(t)
	sources, camara, senado := fakeSources()
	clk := clock.System{}
	uc := newContentUseCases(postgresContentPorts(pool, clk, noScheduler{}, sources), clk)
	editor := auth.WithActor(t.Context(), auth.Actor{UserID: "00000000-0000-4000-8000-0000000000e1", Role: auth.RoleEditor})

	if _, err := uc.ingest.Import.Execute(editor, ingestapp.ImportInput{Chamber: "camara", ExternalID: "2192459"}); err != nil {
		t.Fatal(err)
	}
	soon, yesterday := time.Now().Add(26*time.Hour), time.Now().Add(-24*time.Hour)
	camara.AgendaHits = []ingestapp.RadarHit{
		{Chamber: "camara", ExternalID: "2192459", Kind: "PEC", Number: 221, Year: 2019, AgendaAt: &soon},
		{Chamber: "camara", ExternalID: "2400000", Kind: "PL", Number: 10, Year: 2024, Summary: "Dispõe", AgendaAt: &soon},
	}
	senado.PresentedHits = []ingestapp.RadarHit{{Chamber: "senado", ExternalID: "9100001", Kind: "PEC", Number: 21, Year: 2026, PresentedAt: &yesterday}}
	res, err := uc.scanRadar.Execute(t.Context())
	if err != nil || res.Found != 3 {
		t.Fatalf("rodada = %+v, %v", res, err)
	}
	v, err := uc.ingest.ListRadar.Execute(editor)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Items) != 2 || v.Items[0].ExternalID != "2400000" || v.Items[1].ExternalID != "9100001" || v.LastSeen == nil {
		t.Fatalf("radar = %+v", v)
	}
	if err := uc.ingest.DismissRadar.Execute(editor, "senado", "9100001"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM audit.events WHERE action = 'radar.dismiss'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("auditoria = %d, %v", n, err)
	}
}
