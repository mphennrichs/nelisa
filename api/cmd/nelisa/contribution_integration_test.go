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
