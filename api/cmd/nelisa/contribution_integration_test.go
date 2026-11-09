package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestContributionsEndToEnd: o Leitor manda uma Atualização de tramitação
// de uma Proposição pública e uma Indicação, consulta pelo código, e a
// equipe cadastra e remove uma Fonte oficial.
func TestContributionsEndToEnd(t *testing.T) {
	a := newEditorialApp(t, nil)

	// Proposição ainda "criada" (rascunho): o Leitor não a vê, e a
	// Contribuição é recusada como se ela não existisse.
	var imp struct {
		PropositionID string `json:"proposition_id"`
	}
	w := a.must(a.do("POST", "/api/team/propositions/import", "edu", `{"chamber":"camara","external_id":"2192459"}`), 201, "import")
	if err := json.Unmarshal(w.Body.Bytes(), &imp); err != nil {
		t.Fatal(err)
	}
	update := `{"kind":"progress_update","proposition_slug":"pec-221-2019","link":"https://g1.globo.com/x","event_type":"vote_postponed","event_date":"2026-11-20","note":"adiada"}`
	a.expectErr(a.do("POST", "/api/public/contributions", "", update), 404, "proposition_not_found", "rascunho")

	a.must(a.do("POST", "/api/team/propositions/"+imp.PropositionID+"/submit-for-review", "edu", ""), 200, "submit")
	w = a.must(a.do("POST", "/api/public/contributions", "", update), 201, "contribution")
	var sub struct {
		TrackingCode string `json:"tracking_code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sub)

	// O banco guarda só o hash do código e nada do Leitor além do formulário.
	var stored, propID string
	if err := a.pool.QueryRow(t.Context(), `SELECT tracking_hash, proposition_id::text FROM contribution.contributions`).Scan(&stored, &propID); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, strings.ReplaceAll(sub.TrackingCode, "-", "")) || len(stored) != 64 || propID != imp.PropositionID {
		t.Errorf("guardado: %q %q", stored, propID)
	}

	w = a.must(a.do("POST", "/api/public/contributions/lookup", "", `{"code":"`+sub.TrackingCode+`"}`), 200, "lookup")
	if body := w.Body.String(); !strings.Contains(body, `"state":"pending"`) || !strings.Contains(body, `"proposition_slug":"pec-221-2019"`) || strings.Contains(body, "adiada") {
		t.Errorf("lookup = %s", body)
	}

	// Indicação: só domínio de Fonte oficial.
	a.expectErr(a.do("POST", "/api/public/contributions", "", `{"kind":"indication","link":"https://cmsp.org/pl/1","sphere":"municipal"}`), 400, "not_official_source", "indicação")
	w = a.must(a.do("POST", "/api/team/official-sources", "edu", `{"domain":"cmsp.org","name":"Câmara Municipal de São Paulo","sphere":"municipal"}`), 201, "fonte oficial")
	var src struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &src)
	a.must(a.do("POST", "/api/public/contributions", "", `{"kind":"indication","link":"https://www.cmsp.org/pl/1","sphere":"municipal"}`), 201, "indicação")
	a.expectErr(a.do("POST", "/api/team/official-sources", "rita", `{"domain":"www.cmsp.org","name":"x","sphere":"municipal"}`), 409, "official_source_exists", "repetida")
	a.expectErr(a.do("DELETE", "/api/team/official-sources/"+src.ID, "edu", ""), 403, "forbidden", "editor remove")
	a.must(a.do("DELETE", "/api/team/official-sources/"+src.ID, "ana", ""), 204, "admin remove")
	if n, m := a.auditCount("official_source.create"), a.auditCount("official_source.delete"); n != 1 || m != 1 {
		t.Errorf("auditoria: create %d, delete %d", n, m)
	}
}

// TestContributionModerationEndToEnd: o "pronto quando" da fase 3 sem o
// Turnstile (passo 3). O Leitor manda uma Atualização de tramitação da PEC
// 221/2019; ela aparece na Fila; o Editor aceita com o evento revisado; o
// Leitor vê "aceita" e o evento aparece na Tramitação pública com o link
// da fonte. Depois o Editor apaga o evento manual, e rejeita uma Indicação.
func TestContributionModerationEndToEnd(t *testing.T) {
	a := newEditorialApp(t, nil)
	var imp struct {
		PropositionID string `json:"proposition_id"`
	}
	w := a.must(a.do("POST", "/api/team/propositions/import", "edu", `{"chamber":"camara","external_id":"2192459"}`), 201, "import")
	_ = json.Unmarshal(w.Body.Bytes(), &imp)
	a.must(a.do("POST", "/api/team/propositions/"+imp.PropositionID+"/submit-for-review", "edu", ""), 200, "submit")

	w = a.must(a.do("POST", "/api/public/contributions", "", `{"kind":"progress_update","proposition_slug":"pec-221-2019","link":"https://g1.globo.com/x","event_type":"vote_postponed","event_date":"2026-11-20","note":"<b>adiada</b>"}`), 201, "contribution")
	var sub struct {
		TrackingCode string `json:"tracking_code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sub)

	// Na Fila, sem Posse e com o nome da Proposição.
	var q struct {
		Items []struct {
			PropositionID *string `json:"proposition_id"`
			Name          string
			Reason        string
			Version       *struct{ ID string }
			Contribution  *struct{ ID, Kind string }
		}
	}
	w = a.must(a.do("GET", "/api/team/queue", "edu", ""), 200, "fila")
	_ = json.Unmarshal(w.Body.Bytes(), &q)
	var cid string
	for _, it := range q.Items {
		if it.Reason == "contribution" {
			if it.Contribution == nil || it.Version != nil || it.PropositionID == nil || *it.PropositionID != imp.PropositionID || it.Name != "PEC 221/2019" {
				t.Errorf("item = %+v", it)
			}
			cid = it.Contribution.ID
		}
	}
	if cid == "" {
		t.Fatalf("contribuição fora da Fila: %s", w.Body.String())
	}

	// O detalhe é da equipe; o texto do Leitor vai como veio.
	a.expectErr(a.do("GET", "/api/team/contributions/"+cid, "", ""), 401, "unauthenticated", "anônimo")
	w = a.must(a.do("GET", "/api/team/contributions/"+cid, "edu", ""), 200, "detalhe")
	var det struct {
		LinkHost string `json:"link_host"`
		Note     string
	}
	_ = json.Unmarshal(w.Body.Bytes(), &det)
	if det.LinkHost != "g1.globo.com" || det.Note != "<b>adiada</b>" {
		t.Errorf("detalhe = %s", w.Body.String())
	}

	a.expectErr(a.do("POST", "/api/team/contributions/"+cid+"/accept", "edu", `{}`), 400, "event_required", "sem evento")
	a.expectErr(a.do("PUT", "/api/team/contributions/"+cid+"/sphere", "edu", `{"sphere":"state"}`), 400, "sphere_fixed", "esfera")
	accept := `{"internal_note":"conferido no g1","event":{"chamber":"camara","date":"2026-11-20","description":"Votação adiada no Plenário","source_url":"https://g1.globo.com/x"}}`
	w = a.must(a.do("POST", "/api/team/contributions/"+cid+"/accept", "edu", accept), 200, "aceitar")
	if !strings.Contains(w.Body.String(), `"state":"accepted"`) {
		t.Errorf("aceita = %s", w.Body.String())
	}
	a.expectErr(a.do("POST", "/api/team/contributions/"+cid+"/accept", "rita", accept), 409, "contribution_decided", "de novo")
	if n := a.auditCount("contribution.accept"); n != 1 {
		t.Errorf("auditoria accept = %d", n)
	}

	// O Leitor vê "aceita", nunca a nota interna.
	w = a.must(a.do("POST", "/api/public/contributions/lookup", "", `{"code":"`+sub.TrackingCode+`"}`), 200, "lookup")
	if b := w.Body.String(); !strings.Contains(b, `"state":"accepted"`) || strings.Contains(b, "conferido") {
		t.Errorf("lookup = %s", b)
	}
	// Fora da Fila.
	w = a.must(a.do("GET", "/api/team/queue", "edu", ""), 200, "fila depois")
	if strings.Contains(w.Body.String(), cid) {
		t.Errorf("decidida continua na Fila: %s", w.Body.String())
	}

	// Na Tramitação pública, com a fonte; o id da Contribuição só para a equipe.
	w = a.must(a.do("GET", "/api/public/propositions/pec-221-2019", "", ""), 200, "pública")
	if b := w.Body.String(); !strings.Contains(b, `"manual":true`) || !strings.Contains(b, `"source_url":"https://g1.globo.com/x"`) ||
		!strings.Contains(b, "Votação adiada no Plenário") || strings.Contains(b, "contribution_id") {
		t.Errorf("tramitação pública = %s", b)
	}
	w = a.must(a.do("GET", "/api/team/propositions/"+imp.PropositionID, "edu", ""), 200, "equipe")
	if !strings.Contains(w.Body.String(), `"contribution_id":"`+cid+`"`) {
		t.Errorf("tramitação da equipe sem contribution_id")
	}

	a.must(a.do("DELETE", "/api/team/contributions/"+cid+"/progress-entry", "edu", ""), 204, "apagar evento")
	a.expectErr(a.do("DELETE", "/api/team/contributions/"+cid+"/progress-entry", "edu", ""), 404, "progress_entry_not_found", "apagar de novo")
	w = a.must(a.do("GET", "/api/public/propositions/pec-221-2019", "", ""), 200, "pública depois")
	if strings.Contains(w.Body.String(), "Votação adiada no Plenário") {
		t.Error("evento manual continua na Tramitação")
	}

	// Indicação rejeitada: o Leitor vê o motivo público.
	w = a.must(a.do("POST", "/api/public/contributions", "", `{"kind":"indication","link":"https://www.camara.leg.br/pl/1","sphere":"federal"}`), 201, "indicação")
	_ = json.Unmarshal(w.Body.Bytes(), &sub)
	var ind string
	if err := a.pool.QueryRow(t.Context(), `SELECT id::text FROM contribution.contributions WHERE kind = 'indication'`).Scan(&ind); err != nil {
		t.Fatal(err)
	}
	a.expectErr(a.do("POST", "/api/team/contributions/"+ind+"/reject", "edu", `{"reason":"nao-sei"}`), 400, "invalid_reason", "motivo")
	a.must(a.do("POST", "/api/team/contributions/"+ind+"/reject", "edu", `{"reason":"already_registered","internal_note":"já é a PEC 221"}`), 200, "rejeitar")
	w = a.must(a.do("POST", "/api/public/contributions/lookup", "", `{"code":"`+sub.TrackingCode+`"}`), 200, "lookup indicação")
	if b := w.Body.String(); !strings.Contains(b, `"public_reason":"already_registered"`) || strings.Contains(b, "PEC 221") {
		t.Errorf("lookup = %s", b)
	}
	a.expectErr(a.do("GET", "/api/team/contributions/não-é-uuid", "edu", ""), 404, "contribution_not_found", "id ruim")
}
