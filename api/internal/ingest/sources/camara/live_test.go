package camara

import (
	"context"
	"os"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
)

// TestLive bate na API real. Só roda com NELISA_LIVE_SOURCES=1.
func TestLive(t *testing.T) {
	if os.Getenv("NELISA_LIVE_SOURCES") != "1" {
		t.Skip("defina NELISA_LIVE_SOURCES=1 para testar contra a API real da Câmara")
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
	if len(recs) != 1 || recs[0].ExternalID != "2233802" || recs[0].Role != "origin" {
		t.Fatalf("busca = %+v", recs)
	}
	prog, err := s.Progress(ctx, "2233802")
	if err != nil {
		t.Fatal(err)
	}
	if len(prog) < 400 {
		t.Fatalf("%d eventos de tramitação", len(prog))
	}
	keys := map[string]bool{}
	for _, p := range prog {
		if keys[p.ExternalKey] {
			t.Fatalf("ExternalKey repetida %q", p.ExternalKey)
		}
		keys[p.ExternalKey] = true
	}
	if _, err := s.Fetch(ctx, "2485341"); err != nil {
		t.Fatal(err)
	}
}
