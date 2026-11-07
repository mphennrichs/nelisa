package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"

	ingestapp "github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/app/fakes"
	ingestjobs "github.com/mphennrichs/nelisa/api/internal/ingest/jobs"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/jobs"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
)

var day0 = time.Date(2019, 11, 26, 12, 0, 0, 0, time.UTC)

// fakeSources imita a Câmara e o Senado com a PEC 221/2019 (nas duas
// Casas) e a PEC 8/2025 (só na Câmara, apensada à 221).
func fakeSources() (ingestapp.Sources, *fakes.Source, *fakes.Source) {
	camara, senado := fakes.NewSource(ingestapp.ChamberCamara), fakes.NewSource(ingestapp.ChamberSenado)
	camara.Put(ingestapp.SourceRecord{
		ExternalID: "2192459", Kind: "PEC", Number: 221, Year: 2019, Summary: "Reduz a jornada de trabalho",
		OriginChamber: ingestapp.ChamberCamara, Role: "origin", Status: "Remetida ao Senado Federal",
		SourceURL: "https://www.camara.leg.br/proposicoesWeb/fichadetramitacao?idProposicao=2192459",
		Authors:   []string{"Dep. Reginaldo Lopes (PT/MG)"}, PresentedAt: day0,
	}, fakes.Progress1("1-100", day0, "Apresentação"), fakes.Progress1("2-200", day0.Add(30*24*time.Hour), "Aprovada no Plenário"))
	senado.Put(ingestapp.SourceRecord{
		ExternalID: "8441243", Kind: "PEC", Number: 221, Year: 2019, Role: "review", OriginChamber: ingestapp.ChamberCamara,
		Status: "Aguardando leitura", SourceURL: "https://www25.senado.leg.br/web/atividade/materias/-/materia/8441243",
	}, fakes.Progress1("9001", day0.Add(40*24*time.Hour), "Recebida no Senado"))
	camara.Put(ingestapp.SourceRecord{
		ExternalID: "2487436", Kind: "PEC", Number: 8, Year: 2025, OriginChamber: ingestapp.ChamberCamara,
		AttachedTo: "PEC 221/2019", Status: "Aguardando parecer",
	}, fakes.Progress1("1-100", day0.Add(400*24*time.Hour), "Apresentação"))
	return ingestapp.NewSources(camara, senado), camara, senado
}

// Ponta a ponta com o Postgres real e o River real: Importação pela API,
// sugestão nunca confirmada sozinha, confirmação enfileirando a
// Sincronização na mesma transação, worker sincronizando, regra de
// visibilidade e Auditoria.
func TestContentEndToEnd(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := testConfig()
	cfg.BootstrapToken = "segredo-de-bootstrap-e2e"
	clk := clock.System{}
	sources, _, senado := fakeSources()

	inserter, err := jobs.NewInsertOnlyClient(pool, logger)
	if err != nil {
		t.Fatal(err)
	}
	sched := ingestjobs.NewScheduler(jobs.NewInserter(inserter))
	content := newContentModule(newContentUseCases(postgresContentPorts(pool, clk, sched, sources), clk), logger)
	h := newRouter(cfg, pool, newTeamModule(cfg, postgresTeamPorts(pool, clk), clk, logger), content, logger)

	do := func(method, path, body, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "192.0.2.10:1234"
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	var session struct {
		AccessToken string `json:"access_token"`
	}
	w := do("POST", "/api/team/auth/bootstrap",
		`{"token":"segredo-de-bootstrap-e2e","name":"Ana","email":"ana@nelisa.org","password":"senha-forte"}`, "")
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &session) != nil {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body)
	}
	tok := session.AccessToken

	// Busca nas duas Casas.
	w = do("GET", "/api/team/sources/search?kind=PEC&number=221&year=2019", "", tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "2192459") || !strings.Contains(w.Body.String(), "8441243") {
		t.Fatalf("search: %d %s", w.Code, w.Body)
	}

	// Importação, e de novo (idempotente).
	var imp struct {
		PropositionID string `json:"proposition_id"`
		Created       bool   `json:"created"`
		Suggested     int    `json:"suggested_links"`
	}
	w = do("POST", "/api/team/propositions/import", `{"chamber":"camara","external_id":"2192459"}`, tok)
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &imp) != nil || imp.Suggested != 1 {
		t.Fatalf("import: %d %s", w.Code, w.Body)
	}
	pid := imp.PropositionID
	w = do("POST", "/api/team/propositions/import", `{"chamber":"camara","external_id":"2192459"}`, tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), pid) {
		t.Fatalf("import again: %d %s", w.Code, w.Body)
	}

	type detail struct {
		Slug        string `json:"slug"`
		ReviewState string `json:"review_state"`
		AttachedTo  *struct {
			ID   *string `json:"id"`
			Name string  `json:"name"`
		} `json:"attached_to"`
		Links []struct {
			ID, Chamber, State string
			LastSyncedAt       *string `json:"last_synced_at"`
		} `json:"links"`
		Timeline []struct{ Chamber, Description string } `json:"timeline"`
	}
	get := func(id string) detail {
		t.Helper()
		w := do("GET", "/api/team/propositions/"+id, "", tok)
		var d detail
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &d) != nil {
			t.Fatalf("get: %d %s", w.Code, w.Body)
		}
		return d
	}
	d := get(pid)
	var senadoLink string
	for _, l := range d.Links {
		if l.Chamber == "senado" {
			senadoLink = l.ID
			if l.State != "suggested" {
				t.Fatalf("senado link auto-confirmed: %+v", l)
			}
		}
	}
	if d.Slug != "pec-221-2019" || d.ReviewState != "draft" || len(d.Timeline) != 2 || senadoLink == "" {
		t.Fatalf("after import: %+v", d)
	}

	// Rascunho não existe para o Leitor.
	if w := do("GET", "/api/public/propositions/pec-221-2019", "", ""); w.Code != 404 {
		t.Fatalf("draft public: %d %s", w.Code, w.Body)
	}

	// Confirmar enfileira a Sincronização na mesma transação.
	if w := do("POST", "/api/team/propositions/"+pid+"/links/"+senadoLink+"/confirm", "", tok); w.Code != 200 {
		t.Fatalf("confirm: %d %s", w.Code, w.Body)
	}
	rivertest.RequireInserted(ctx, t, riverpgxv5.New(pool), ingestjobs.SyncLinkArgs{LinkID: senadoLink},
		&rivertest.RequireInsertedOpts{Schema: jobs.Schema, MaxAttempts: 8})

	// O worker de verdade sincroniza (e a rodada periódica roda ao subir).
	worker, err := newWorkerClient(pool, sources, time.Hour, clk, logger)
	if err != nil {
		t.Fatal(err)
	}
	done, cancel := worker.Subscribe(river.EventKindJobCompleted)
	defer cancel()
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(30 * time.Second)
	for len(get(pid).Timeline) < 3 {
		select {
		case <-done:
		case <-deadline:
			t.Fatalf("worker did not sync: %+v", get(pid))
		case <-time.After(200 * time.Millisecond):
		}
	}
	stopCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	if err := worker.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if senado.Calls["fetch"] == 0 {
		t.Error("senado was never fetched")
	}

	// Sincronizar de novo não duplica a Tramitação.
	uc := newContentUseCases(postgresContentPorts(pool, clk, sched, sources), clk)
	for range 2 {
		if _, err := uc.syncLink.Execute(ctx, senadoLink); err != nil {
			t.Fatal(err)
		}
	}
	d = get(pid)
	if len(d.Timeline) != 3 || d.Timeline[2].Chamber != "senado" {
		t.Fatalf("timeline = %+v", d.Timeline)
	}
	for _, l := range d.Links {
		if l.State != "confirmed" || l.LastSyncedAt == nil {
			t.Errorf("link not synced: %+v", l)
		}
	}

	// Em análise: público, só dados oficiais.
	if w := do("POST", "/api/team/propositions/"+pid+"/submit-for-review", "", tok); w.Code != 200 {
		t.Fatalf("submit: %d %s", w.Code, w.Body)
	}
	w = do("GET", "/api/public/propositions/pec-221-2019", "", "")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `"under_review":true`) || !strings.Contains(body, "Recebida no Senado") {
		t.Fatalf("public: %d %s", w.Code, body)
	}
	for _, leak := range []string{"review_state", "last_sync_error", "confirmed_by", pid, senadoLink} {
		if strings.Contains(body, leak) {
			t.Errorf("public leaks %q", leak)
		}
	}

	// A apensada chega depois e acha a principal pelo nome oficial.
	w = do("POST", "/api/team/propositions/import", `{"chamber":"camara","external_id":"2487436"}`, tok)
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &imp) != nil {
		t.Fatalf("import attached: %d %s", w.Code, w.Body)
	}
	if a := get(imp.PropositionID); a.AttachedTo == nil || a.AttachedTo.ID == nil || *a.AttachedTo.ID != pid {
		t.Errorf("attachment not resolved: %+v", a.AttachedTo)
	}

	// Auditoria de cada ação da equipe.
	rows, err := pool.Query(ctx, `SELECT action, count(*) FROM audit.events GROUP BY action`)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for rows.Next() {
		var a string
		var n int
		if err := rows.Scan(&a, &n); err != nil {
			t.Fatal(err)
		}
		counts[a] = n
	}
	rows.Close()
	if counts["proposition.import"] != 2 || counts["source_link.confirm"] != 1 || counts["proposition.submit_for_review"] != 1 {
		t.Errorf("audit = %v", counts)
	}
}
