package main

import (
	"context"
	"encoding/json"
	"fmt"
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
// worker de verdade lê, quebra em Dispositivos e grava. O Editor corrige o
// texto, confirma, e só então a Versão pode ser aprovada. O texto extraído
// nunca sai no público.
func TestStructuringEndToEnd(t *testing.T) {
	a := newEditorialAppWith(t, nil, true)
	ctx := context.Background()
	var imp struct {
		PropositionID string `json:"proposition_id"`
	}
	w := a.must(a.do("POST", "/api/team/propositions/import", "edu", `{"chamber":"camara","external_id":"2623604"}`), 201, "import")
	_ = json.Unmarshal(w.Body.Bytes(), &imp)
	pid := imp.PropositionID
	a.must(a.do("POST", "/api/team/propositions/"+pid+"/submit-for-review", "edu", ""), 200, "submit")
	vid, _ := a.pending(pid)

	rivertest.RequireInserted(ctx, t, riverpgxv5.New(a.pool), propjobs.StructureVersionArgs{VersionID: vid},
		&rivertest.RequireInsertedOpts{Schema: jobs.Schema, MaxAttempts: 5})
	if st := a.structuring(pid, "edu"); st.State != "fetching" {
		t.Fatalf("state = %s", st.State)
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
	for a.structuring(pid, "edu").State == "fetching" {
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

	st := a.structuring(pid, "edu")
	if st.State != "ready_to_check" || st.Failure != nil || st.UpdatedBy != nil {
		t.Fatalf("estruturação = %+v", st)
	}
	if got := st.addresses(); got != "ementa preambulo art-1 art-2 justificacao" {
		t.Fatalf("provisions = %s", got)
	}
	if strings.Contains(st.SourceText, "PEC n.221/2019") {
		t.Errorf("rodapé repetido ficou no texto: %q", st.SourceText)
	}

	// Conteúdo completo, mas a Estruturação a conferir ainda bloqueia.
	trab := a.topic("Trabalho")
	_, rev := a.pending(pid)
	a.must(a.do("PUT", "/api/team/propositions/"+pid+"/versions/"+vid+"/content", "edu",
		contentBody("Reduz a idade penal.", "Idade penal.", []string{trab}, rev)), 200, "content")
	if d := a.detail(pid, "rita"); d.CanApprove || !hasBlocker(d, "structuring_pending") {
		t.Fatalf("rita: %+v", d)
	}

	// Rui (sem a Posse) não mexe; Edu com revisão velha leva 409.
	fixed := strings.Replace(st.SourceText, "Art. 2º", "Art. 2º O texto corrigido.", 1)
	body := func(text string, rev int) string {
		b, _ := json.Marshal(map[string]any{"source_text": text, "revision": rev})
		return string(b)
	}
	path := "/api/team/propositions/" + pid + "/versions/" + vid + "/structuring"
	a.expectErr(a.do("PUT", path, "rui", body(fixed, st.Revision)), 409, "claim_taken", "rui reprocess")
	a.expectErr(a.do("PUT", path, "edu", body(fixed, st.Revision-1)), 409, "stale_content", "stale reprocess")
	a.expectErr(a.do("PUT", path, "edu", body("  ", st.Revision)), 400, "invalid_source_text", "empty text")
	a.expectErr(a.do("POST", path+"/retry", "edu", ""), 409, "structuring_cannot_retry", "retry")

	var after structuringView
	w = a.must(a.do("PUT", path, "edu", body(fixed, st.Revision)), 200, "reprocess")
	_ = json.Unmarshal(w.Body.Bytes(), &after)
	if after.State != "ready_to_check" || after.Revision != st.Revision+1 || !strings.Contains(after.SourceText, "corrigido") {
		t.Fatalf("depois de corrigir: %+v", after)
	}
	a.must(a.do("POST", path+"/confirm", "edu", fmt.Sprintf(`{"revision":%d}`, after.Revision)), 200, "confirm")
	a.expectErr(a.do("POST", path+"/confirm", "edu", fmt.Sprintf(`{"revision":%d}`, after.Revision+1)), 409, "structuring_not_ready", "confirm twice")

	// Confirmada: Edu comenta o art. 1º; o texto não pode mais perder o
	// art. 1º enquanto o Comentário existir.
	cpath := "/api/team/propositions/" + pid + "/versions/" + vid + "/comments"
	a.expectErr(a.do("POST", cpath, "edu", `{"provision_address":"art-9","body":"x"}`), 400, "provision_not_found", "unknown provision")
	a.must(a.do("POST", cpath, "edu", `{"provision_address":"art-1","body":"COMENTARIO-PUBLICO sobre o art. 1º."}`), 201, "comment")
	var cur structuringView
	_ = json.Unmarshal(a.must(a.do("GET", "/api/team/propositions/"+pid, "edu", ""), 200, "detail").Body.Bytes(), &struct {
		P *structuringView `json:"pending_structuring"`
	}{&cur})
	noArt1 := strings.Replace(cur.SourceText, "Art. 1º", "Texto solto", 1)
	a.expectErr(a.do("PUT", path, "edu", body(noArt1, cur.Revision)), 409, "anchors_removed", "drop commented provision")
	if pub := a.must(a.do("GET", "/api/public/propositions/pec-9-2026", "", ""), 200, "public").Body.String(); strings.Contains(pub, "COMENTARIO-PUBLICO") ||
		!strings.Contains(pub, `"annotated_text":null`) {
		t.Errorf("rascunho de Comentário vazou: %s", pub)
	}

	// Rita aprova (Edu, autor do texto e do Comentário, não aprovaria).
	d := a.detail(pid, "rita")
	if !d.CanApprove {
		t.Fatalf("rita: %+v", d)
	}
	a.must(a.do("POST", "/api/team/propositions/"+pid+"/versions/"+vid+"/approve", "rita", a.revBody(pid)), 200, "approve")
	if n := a.auditCount("structuring.reprocess"); n != 1 {
		t.Errorf("structuring.reprocess = %d", n)
	}
	if n := a.auditCount("structuring.confirm"); n != 1 {
		t.Errorf("structuring.confirm = %d", n)
	}

	if n := a.auditCount("comment.create"); n != 1 {
		t.Errorf("comment.create = %d", n)
	}

	// Pronta: o Leitor vê o Documento comentado da vigente, sem o texto
	// fonte nem nomes da equipe.
	pub := a.must(a.do("GET", "/api/public/propositions/pec-9-2026", "", ""), 200, "public").Body.String()
	var pd struct {
		AnnotatedText []struct {
			Address  string `json:"address"`
			Text     string `json:"text"`
			Comments []struct {
				Body string `json:"body"`
			} `json:"comments"`
		} `json:"annotated_text"`
	}
	if err := json.Unmarshal([]byte(pub), &pd); err != nil || len(pd.AnnotatedText) != 5 {
		t.Fatalf("annotated_text: %v %s", err, pub)
	}
	if a1 := pd.AnnotatedText[2]; a1.Address != "art-1" || len(a1.Comments) != 1 || !strings.HasPrefix(a1.Comments[0].Body, "COMENTARIO-PUBLICO") {
		t.Fatalf("art-1: %+v", a1)
	}
	for _, leak := range []string{"source_text", "warnings", "Edu", "edu@", a.ids["edu"], "PEC n.221/2019"} {
		if strings.Contains(pub, leak) {
			t.Errorf("vazou %q no público: %s", leak, pub)
		}
	}
}

type structuringView struct {
	State      string  `json:"state"`
	Failure    *string `json:"failure"`
	SourceText string  `json:"source_text"`
	Revision   int     `json:"revision"`
	UpdatedBy  *string `json:"updated_by"`
	Provisions []struct {
		Address string `json:"address"`
	} `json:"provisions"`
}

func (s structuringView) addresses() string {
	var out []string
	for _, p := range s.Provisions {
		out = append(out, p.Address)
	}
	return strings.Join(out, " ")
}

func (a *editorialApp) structuring(pid, who string) structuringView {
	a.t.Helper()
	w := a.must(a.do("GET", "/api/team/propositions/"+pid, who, ""), 200, "detail")
	var d struct {
		PendingStructuring *structuringView `json:"pending_structuring"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil || d.PendingStructuring == nil {
		a.t.Fatalf("sem pending_structuring: %s", w.Body)
	}
	return *d.PendingStructuring
}

func hasBlocker(d teamDetail, code string) bool {
	for _, b := range d.ApproveBlockers {
		if b.Code == code {
			return true
		}
	}
	return false
}
