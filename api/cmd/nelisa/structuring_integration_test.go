package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/jobs"
	propjobs "github.com/mphennrichs/nelisa/api/internal/proposition/jobs"
)

// fakeOfficialText faz o papel do PDF oficial: nenhum teste sai para a rede.
type fakeOfficialText struct{}

func (fakeOfficialText) Fetch(context.Context, string) ([]string, error) {
	return []string{
		"CÂMARA DOS DEPUTADOS\nPROPOSTA DE EMENDA À CONSTITUIÇÃO Nº , DE 2019\n(Do Sr. Fulano)\n" +
			"Reduz a jornada de trabalho.\nAs Mesas da Câmara dos Deputados e do Senado Federal promulgam a seguinte Emenda:\n" +
			"Art. 1º O art. 7º passa a vigorar com alterações.\nPEC n.221/2019\n",
		"CÂMARA DOS DEPUTADOS\nArt. 2º Esta Emenda entra em vigor na data de sua publicação.\n" +
			"JUSTIFICAÇÃO\nA jornada é longa.\nPEC n.221/2019\n",
	}, nil
}

// A Importação enfileira a busca do PDF oficial na mesma transação; o
// worker de verdade lê, quebra em Dispositivos e grava. O texto extraído
// nunca sai no público.
func TestStructuringEndToEnd(t *testing.T) {
	a := newEditorialAppWith(t, nil, true)
	ctx := context.Background()
	pid := a.importAndSubmit()
	vid, _ := a.pending(pid)

	rivertest.RequireInserted(ctx, t, riverpgxv5.New(a.pool), propjobs.StructureVersionArgs{VersionID: vid},
		&rivertest.RequireInsertedOpts{Schema: jobs.Schema, MaxAttempts: 5})
	state := func() string {
		var s string
		if err := a.pool.QueryRow(ctx, `SELECT state FROM proposition.structurings WHERE version_id = $1`, vid).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := state(); s != "fetching" {
		t.Fatalf("state = %s", s)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sources, _, _ := fakeSources()
	worker, err := newWorkerClient(a.pool, sources, fakeOfficialText{}, time.Hour, clock.System{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	done, cancel := worker.Subscribe(river.EventKindJobCompleted)
	defer cancel()
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(30 * time.Second)
	for state() == "fetching" {
		select {
		case <-done:
		case <-deadline:
			t.Fatal("worker did not structure the version")
		case <-time.After(200 * time.Millisecond):
		}
	}
	stopCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	if err := worker.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}

	if s := state(); s != "ready_to_check" {
		t.Fatalf("state = %s", s)
	}
	rows, err := a.pool.Query(ctx, `SELECT address FROM proposition.provisions WHERE version_id = $1 ORDER BY position`, vid)
	if err != nil {
		t.Fatal(err)
	}
	var addrs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		addrs = append(addrs, s)
	}
	if got := strings.Join(addrs, " "); got != "ementa preambulo art-1 art-2 justificacao" {
		t.Fatalf("provisions = %s", got)
	}
	var text string
	if err := a.pool.QueryRow(ctx, `SELECT source_text FROM proposition.structurings WHERE version_id = $1`, vid).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "PEC n.221/2019") {
		t.Errorf("rodapé repetido ficou no texto: %q", text)
	}
	if pub := a.public(); strings.Contains(pub, "promulgam") || strings.Contains(pub, "source_text") {
		t.Errorf("texto da Estruturação vazou no público: %s", pub)
	}
}
