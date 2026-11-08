package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

const (
	pid   = "11111111-1111-4111-8111-111111111111"
	vCur  = "22222222-2222-4222-8222-222222222221"
	vPend = "22222222-2222-4222-8222-222222222222"
)

// seedEditorial deixa a PEC 221/2019 com uma Versão vigente aprovada (Selo
// 1) e uma em análise cheia de segredos.
func (s *server) seedEditorial(t *testing.T) {
	t.Helper()
	s.seed(t, domain.ReviewInReview)
	s.w.PutTopic(domain.Topic{ID: "t-trab", Name: "Trabalho", Active: true, CreatedAt: t0})
	s.w.PutTopic(domain.Topic{ID: "t-sec", Name: "SEGREDO-TEMA", Active: true, CreatedAt: t0})
	s.w.PutTopic(domain.Topic{ID: "t-old", Name: "Antigo", Active: false, CreatedAt: t0})
	s.w.PutVersion(domain.Version{ID: vCur, PropositionID: pid, Seq: 1, Label: "Texto original", OfficialTextURL: "https://www.camara.leg.br/v1", State: domain.VersionCurrent, CreatedAt: t0})
	s.w.PutContent(domain.Content{VersionID: vCur, Summary: "Resumo aprovado", ShortSummary: "Curto aprovado", TopicIDs: []string{"t-trab"}, Authors: []string{"SEGREDO-AUTOR"}, Revision: 4, UpdatedBy: "SEGREDO-AUTOR", UpdatedAt: t0})
	s.w.PutVersion(domain.Version{ID: vPend, PropositionID: pid, Seq: 2, Label: "Substitutivo", OfficialTextURL: "https://www.camara.leg.br/v2", State: domain.VersionPending, CreatedBy: "SEGREDO-CRIADOR", CreatedAt: t0})
	s.w.PutContent(domain.Content{VersionID: vPend, Summary: "SEGREDO-RESUMO", ShortSummary: "SEGREDO-CURTO", TopicIDs: []string{"t-sec"}, NeedsConfirmation: true, Authors: []string{"SEGREDO-AUTOR"}, Revision: 9, UpdatedBy: "SEGREDO-AUTOR", UpdatedAt: t0})
	s.w.Badges[vCur] = 1
	s.w.Names = map[string]string{"ana": "Ana Editora", "bia": "Bia"}
}

func TestPublicEditorialFieldsAndNoPendingLeak(t *testing.T) {
	s := newServer(t)
	s.seedEditorial(t)
	s.w.SetClaim(pid, &app.ClaimInfo{UserID: "ana", HolderName: "Ana Editora"})
	for _, path := range []string{"/api/public/propositions/pec-221-2019", "/api/public/propositions"} {
		w := s.do("GET", path, "")
		body := w.Body.String()
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, body)
		}
		for _, want := range []string{`"summary":"Resumo aprovado"`, `"short_summary":"Curto aprovado"`, `"topics":["Trabalho"]`,
			`"review_badge":1`, `"pending_version":{"label":"Substitutivo","official_text_url":"https://www.camara.leg.br/v2"}`,
			`"under_review":false`, `"official_text_url":"https://www.camara.leg.br/v1"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: missing %s in %s", path, want, body)
			}
		}
		// Nada da Versão em análise, nem Autoria, quem aprovou, Posse, "a
		// confirmar" ou revisão.
		for _, leak := range []string{"SEGREDO", "authors\":[\"SEGREDO", "Ana Editora", "needs_confirmation", "revision",
			"approved_by", "claim", vPend, vCur, "created_by", "updated_by"} {
			if strings.Contains(body, leak) {
				t.Errorf("%s leaks %q: %s", path, leak, body)
			}
		}
	}
}

func TestPublicWithoutCurrentHasNullEditorialFields(t *testing.T) {
	s := newServer(t)
	s.seed(t, domain.ReviewInReview)
	s.w.PutVersion(domain.Version{ID: vPend, PropositionID: pid, Seq: 1, Label: "Texto original", State: domain.VersionPending, CreatedAt: t0})
	s.w.PutContent(domain.Content{VersionID: vPend, Summary: "SEGREDO-RESUMO", UpdatedAt: t0})
	w := s.do("GET", "/api/public/propositions/pec-221-2019", "")
	body := w.Body.String()
	for _, want := range []string{`"summary":""`, `"short_summary":""`, `"topics":[]`, `"review_badge":null`, `"pending_version":null`, `"under_review":true`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "SEGREDO") {
		t.Errorf("leak: %s", body)
	}
}

func TestTeamDetailCarriesEditorialDecisions(t *testing.T) {
	s := newServer(t)
	s.seedEditorial(t)
	s.w.Statuses[pid] = app.ReviewStatus{
		CanApprove: false, Blockers: []app.ReviewBlocker{{Code: "needs_confirmation", Message: "Confirme."}},
		Current: &app.ApprovalInfo{VersionID: vCur, ApprovedBy: "bia", ApprovedAt: t0, FourEyes: false, Badge: 1, PendingSecondReview: true},
	}
	s.w.SetClaim(pid, &app.ClaimInfo{UserID: "ana", HolderName: "Ana Editora", ClaimedAt: t0, LastActivityAt: t0, ExpiresAt: t0})
	w := s.do("GET", "/api/team/propositions/"+pid, "bia:editor")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var d struct {
		Versions       []struct{ ID, State string }
		PendingContent *struct {
			Summary           string
			Revision          int
			NeedsConfirmation bool `json:"needs_confirmation"`
		} `json:"pending_content"`
		CurrentContent *struct{ Summary string } `json:"current_content"`
		Claim          *struct {
			HolderName string `json:"holder_name"`
			Mine       bool
		} `json:"claim"`
		CanEdit         bool                    `json:"can_edit"`
		CanApprove      bool                    `json:"can_approve"`
		ApproveBlockers []struct{ Code string } `json:"approve_blockers"`
		ReviewBadge     *int                    `json:"review_badge"`
		CurrentApproval *struct {
			PendingSecondReview bool `json:"pending_second_review"`
		} `json:"current_approval"`
		People map[string]string
		Topics []struct {
			Name   string
			Active bool
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Versions) != 2 || d.PendingContent == nil || d.PendingContent.Revision != 9 || !d.PendingContent.NeedsConfirmation ||
		d.CurrentContent == nil || d.CurrentContent.Summary != "Resumo aprovado" {
		t.Errorf("versions/contents: %s", w.Body)
	}
	if d.Claim == nil || d.Claim.HolderName != "Ana Editora" || d.Claim.Mine || d.CanEdit {
		t.Errorf("claim held by someone else: %s", w.Body)
	}
	if d.CanApprove || len(d.ApproveBlockers) != 1 || d.ReviewBadge == nil || *d.ReviewBadge != 1 ||
		d.CurrentApproval == nil || !d.CurrentApproval.PendingSecondReview {
		t.Errorf("review: %s", w.Body)
	}
	if d.People["bia"] != "Bia" || len(d.Topics) != 3 {
		t.Errorf("people/topics: %s", w.Body)
	}
	w = s.do("GET", "/api/team/propositions/"+pid, "ana:editor")
	if !strings.Contains(w.Body.String(), `"can_edit":true`) || !strings.Contains(w.Body.String(), `"mine":true`) {
		t.Errorf("holder: %s", w.Body)
	}
}

func TestContentRoutes(t *testing.T) {
	s := newServer(t)
	s.seedEditorial(t)
	path := "/api/team/propositions/" + pid + "/versions/" + vPend + "/content"
	body := `{"summary":"Novo","short_summary":"Curto","topic_ids":["t-trab"],"revision":9}`

	if w := s.doJSON("PUT", path, "", body); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
	if w := s.doJSON("PUT", path, "ana:editor", `{"summary":"x"}`); w.Code != 400 || errCode(t, w) != "invalid_json" {
		t.Errorf("missing revision: %d %s", w.Code, w.Body)
	}
	if w := s.doJSON("PUT", path, "ana:editor", `{"summary":"x","revision":9,"extra":1}`); w.Code != 400 {
		t.Errorf("unknown field: %d", w.Code)
	}
	if w := s.doJSON("PUT", path, "ana:editor", `{"summary":"x","revision":1}`); w.Code != 409 || errCode(t, w) != "stale_content" {
		t.Errorf("stale: %d %s", w.Code, w.Body)
	}
	long := `{"short_summary":"` + strings.Repeat("a", 281) + `","revision":9}`
	if w := s.doJSON("PUT", path, "ana:editor", long); w.Code != 400 || errCode(t, w) != "short_summary_too_long" {
		t.Errorf("long short summary: %d %s", w.Code, w.Body)
	}
	if w := s.doJSON("PUT", path, "ana:editor", `{"topic_ids":["t-old"],"revision":9}`); w.Code != 400 || errCode(t, w) != "topic_inactive" {
		t.Errorf("inactive topic: %d %s", w.Code, w.Body)
	}
	w := s.doJSON("PUT", path, "ana:editor", body)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"revision":10`) || !strings.Contains(w.Body.String(), `"needs_confirmation":true`) {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	// Bia tenta gravar com a Posse da Ana.
	w = s.doJSON("PUT", path, "bia:editor", strings.Replace(body, `"revision":9`, `"revision":10`, 1))
	if w.Code != 409 || errCode(t, w) != "claim_taken" || !strings.Contains(w.Body.String(), "Ana Editora") {
		t.Errorf("claim taken: %d %s", w.Code, w.Body)
	}
	w = s.doJSON("POST", path+"/confirm", "ana:editor", `{"revision":10}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"needs_confirmation":false`) {
		t.Errorf("confirm: %d %s", w.Code, w.Body)
	}
	if w := s.doJSON("POST", path+"/confirm", "ana:editor", `{"revision":11}`); w.Code != 409 || errCode(t, w) != "nothing_to_confirm" {
		t.Errorf("confirm twice: %d %s", w.Code, w.Body)
	}
	// Conteúdo da vigente não se edita.
	cur := "/api/team/propositions/" + pid + "/versions/" + vCur + "/content"
	if w := s.doJSON("PUT", cur, "ana:editor", `{"revision":4}`); w.Code != 409 || errCode(t, w) != "version_not_editable" {
		t.Errorf("current: %d %s", w.Code, w.Body)
	}
	if w := s.doJSON("PUT", "/api/team/propositions/"+pid+"/versions/nao-existe/content", "ana:editor", `{"revision":0}`); w.Code != 404 || errCode(t, w) != "version_not_found" {
		t.Errorf("unknown version: %d %s", w.Code, w.Body)
	}
}

func TestCreateVersionRoute(t *testing.T) {
	s := newServer(t)
	s.seedEditorial(t)
	path := "/api/team/propositions/" + pid + "/versions"
	if w := s.doJSON("POST", path, "ana:editor", `{"label":"Emenda","official_text_url":"https://x.leg.br"}`); w.Code != 409 || errCode(t, w) != "pending_version_exists" {
		t.Errorf("with pending: %d %s", w.Code, w.Body)
	}
	// Sem a pendente, cria.
	pend := s.w.VersionsOf(pid)[1]
	pend.State = domain.VersionSuperseded
	s.w.PutVersion(pend)
	if w := s.doJSON("POST", path, "ana:editor", `{"label":"","official_text_url":"https://x.leg.br"}`); w.Code != 400 || errCode(t, w) != "invalid_label" {
		t.Errorf("empty label: %d %s", w.Code, w.Body)
	}
	if w := s.doJSON("POST", path, "ana:editor", `{"label":"Emenda","official_text_url":"javascript:x"}`); w.Code != 400 || errCode(t, w) != "invalid_official_text_url" {
		t.Errorf("bad url: %d %s", w.Code, w.Body)
	}
	w := s.doJSON("POST", path, "ana:editor", `{"label":"Emenda","official_text_url":"https://x.leg.br/e"}`)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"seq":3`) || !strings.Contains(w.Body.String(), `"needs_confirmation":true`) {
		t.Errorf("create: %d %s", w.Code, w.Body)
	}
}

func TestTopicRoutes(t *testing.T) {
	s := newServer(t)
	if w := s.doJSON("POST", "/api/team/topics", "rev:reviewer", `{"name":"Saúde"}`); w.Code != 403 {
		t.Errorf("reviewer creates: %d", w.Code)
	}
	w := s.doJSON("POST", "/api/team/topics", "adm:admin", `{"name":"Saúde"}`)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var tp struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &tp)
	if w := s.doJSON("POST", "/api/team/topics", "adm:admin", `{"name":"saúde"}`); w.Code != 409 || errCode(t, w) != "topic_name_taken" {
		t.Errorf("dup: %d %s", w.Code, w.Body)
	}
	if w := s.doJSON("POST", "/api/team/topics", "adm:admin", `{"name":"  "}`); w.Code != 400 || errCode(t, w) != "invalid_topic_name" {
		t.Errorf("blank: %d %s", w.Code, w.Body)
	}
	if w := s.doJSON("PATCH", "/api/team/topics/"+tp.ID, "ana:editor", `{"active":false}`); w.Code != 403 {
		t.Errorf("editor patch: %d", w.Code)
	}
	if w := s.doJSON("PATCH", "/api/team/topics/"+tp.ID, "adm:admin", `{"active":false}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"active":false`) {
		t.Errorf("deactivate: %d %s", w.Code, w.Body)
	}
	if w := s.doJSON("PATCH", "/api/team/topics/nope", "adm:admin", `{"active":true}`); w.Code != 404 || errCode(t, w) != "topic_not_found" {
		t.Errorf("unknown: %d %s", w.Code, w.Body)
	}
	if w := s.do("GET", "/api/team/topics", "ana:editor"); w.Code != 200 || !strings.Contains(w.Body.String(), "Saúde") {
		t.Errorf("list: %d %s", w.Code, w.Body)
	}
}
