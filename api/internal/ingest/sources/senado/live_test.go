package senado

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
)

// TestLive bate na API real. Só roda com NELISA_LIVE_SOURCES=1.
func TestLive(t *testing.T) {
	if os.Getenv("NELISA_LIVE_SOURCES") != "1" {
		t.Skip("defina NELISA_LIVE_SOURCES=1 para testar contra a API real do Senado")
	}
	s, err := New(nil, "", Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	recs, err := s.Search(ctx, app.SearchQuery{Kind: "PEC", Number: 221, Year: 2019})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].ExternalID != "9056435" || recs[0].Role != "review" || recs[0].OriginChamber != app.ChamberCamara {
		t.Fatalf("busca = %+v", recs)
	}
	prog, err := s.Progress(ctx, "9056435")
	if err != nil {
		t.Fatal(err)
	}
	if len(prog) < 40 {
		t.Fatalf("%d eventos de tramitação", len(prog))
	}
	if _, err := s.Search(ctx, app.SearchQuery{Kind: "PEC", Number: 8, Year: 2025}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("PEC 8/2025 no Senado: %v", err)
	}
}
