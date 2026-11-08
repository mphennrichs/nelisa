package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	auditapp "github.com/mphennrichs/nelisa/api/internal/audit/app"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
)

// editorialApp é o sistema montado como em produção (Postgres real), com a
// equipe cadastrada: Ana (Admin), Edu (Editor), Rita e Rui (Revisores).
type editorialApp struct {
	t      *testing.T
	pool   *pgxpool.Pool
	h      http.Handler
	tokens map[string]string
	ids    map[string]string
	ip     int
}

func newEditorialApp(t *testing.T, wrapAudit func(recordAudit) recordAudit) *editorialApp {
	t.Helper()
	pool, _ := testdb.New(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := testConfig()
	cfg.BootstrapToken = "segredo-de-bootstrap-e2e"
	clk := clock.System{}
	sources, _, _ := fakeSources()
	ports := postgresContentPorts(pool, clk, noScheduler{}, sources)
	if wrapAudit != nil {
		ports.audit = wrapAudit(ports.audit)
	}
	content := newContentModule(newContentUseCases(ports, clk), logger)
	a := &editorialApp{
		t: t, pool: pool, tokens: map[string]string{}, ids: map[string]string{},
		h: newRouter(cfg, pool, newTeamModule(cfg, postgresTeamPorts(pool, clk), clk, logger), content, logger),
	}
	var s struct {
		AccessToken string `json:"access_token"`
		User        struct{ ID string }
	}
	w := a.do("POST", "/api/team/auth/bootstrap", "",
		`{"token":"segredo-de-bootstrap-e2e","name":"Ana","email":"ana@nelisa.org","password":"senha-forte"}`)
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &s) != nil {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body)
	}
	a.tokens["ana"], a.ids["ana"] = s.AccessToken, s.User.ID
	for _, u := range []struct{ name, role string }{{"edu", "editor"}, {"rita", "reviewer"}, {"rui", "reviewer"}} {
		body := fmt.Sprintf(`{"name":%q,"email":"%s@nelisa.org","password":"senha-forte","role":%q}`, strings.ToUpper(u.name[:1])+u.name[1:], u.name, u.role)
		var created struct{ ID string }
		if w := a.do("POST", "/api/team/users", "ana", body); w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &created) != nil {
			t.Fatalf("create %s: %d %s", u.name, w.Code, w.Body)
		}
		w := a.do("POST", "/api/team/auth/login", "", fmt.Sprintf(`{"email":"%s@nelisa.org","password":"senha-forte"}`, u.name))
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &s) != nil {
			t.Fatalf("login %s: %d %s", u.name, w.Code, w.Body)
		}
		a.tokens[u.name], a.ids[u.name] = s.AccessToken, created.ID
	}
	return a
}

func (a *editorialApp) do(method, path, who, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	a.ip++ // cada chamada de um IP diferente: o limite do login não atrapalha
	r.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", a.ip%250+1)
	if who != "" {
		r.Header.Set("Authorization", "Bearer "+a.tokens[who])
	}
	w := httptest.NewRecorder()
	a.h.ServeHTTP(w, r)
	return w
}

func (a *editorialApp) must(w *httptest.ResponseRecorder, status int, what string) *httptest.ResponseRecorder {
	a.t.Helper()
	if w.Code != status {
		a.t.Fatalf("%s: %d %s", what, w.Code, w.Body)
	}
	return w
}

func (a *editorialApp) expectErr(w *httptest.ResponseRecorder, status int, code, what string) {
	a.t.Helper()
	var e struct {
		Error struct{ Code, Message string }
	}
	if w.Code != status || json.Unmarshal(w.Body.Bytes(), &e) != nil || e.Error.Code != code {
		a.t.Fatalf("%s: want %d %s, got %d %s", what, status, code, w.Code, w.Body)
	}
}

type teamDetail struct {
	ReviewState string `json:"review_state"`
	Versions    []struct {
		ID, State string
		Seq       int
	}
	PendingContent *struct {
		Summary           string
		Revision          int
		Authors           []string
		NeedsConfirmation bool `json:"needs_confirmation"`
	} `json:"pending_content"`
	Claim *struct {
		HolderName string `json:"holder_name"`
	}
	CanEdit         bool                    `json:"can_edit"`
	CanApprove      bool                    `json:"can_approve"`
	ApproveBlockers []struct{ Code string } `json:"approve_blockers"`
	CanSecondReview bool                    `json:"can_second_review"`
	ReviewBadge     *int                    `json:"review_badge"`
}

func (a *editorialApp) detail(pid, who string) teamDetail {
	a.t.Helper()
	w := a.must(a.do("GET", "/api/team/propositions/"+pid, who, ""), 200, "detail")
	var d teamDetail
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		a.t.Fatal(err)
	}
	return d
}

func (a *editorialApp) pending(pid string) (string, int) {
	a.t.Helper()
	d := a.detail(pid, "ana")
	for _, v := range d.Versions {
		if v.State == "pending" {
			return v.ID, d.PendingContent.Revision
		}
	}
	a.t.Fatalf("no pending version: %+v", d)
	return "", 0
}

func (a *editorialApp) importAndSubmit() string {
	a.t.Helper()
	var imp struct {
		PropositionID string `json:"proposition_id"`
	}
	w := a.must(a.do("POST", "/api/team/propositions/import", "edu", `{"chamber":"camara","external_id":"2192459"}`), 201, "import")
	_ = json.Unmarshal(w.Body.Bytes(), &imp)
	a.must(a.do("POST", "/api/team/propositions/"+imp.PropositionID+"/submit-for-review", "edu", ""), 200, "submit")
	return imp.PropositionID
}

func (a *editorialApp) topic(name string) string {
	a.t.Helper()
	var tp struct{ ID string }
	w := a.must(a.do("POST", "/api/team/topics", "ana", `{"name":"`+name+`"}`), 201, "topic")
	_ = json.Unmarshal(w.Body.Bytes(), &tp)
	return tp.ID
}

func contentBody(summary, short string, topics []string, rev int) string {
	b, _ := json.Marshal(map[string]any{"summary": summary, "short_summary": short, "topic_ids": topics, "revision": rev})
	return string(b)
}

func (a *editorialApp) public() string {
	a.t.Helper()
	detail := a.must(a.do("GET", "/api/public/propositions/pec-221-2019", "", ""), 200, "public").Body.String()
	list := a.must(a.do("GET", "/api/public/propositions", "", ""), 200, "public list").Body.String()
	return detail + "\n" + list
}

func (a *editorialApp) auditCount(action string) int {
	a.t.Helper()
	var n int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit.events WHERE action = $1`, action).Scan(&n); err != nil {
		a.t.Fatal(err)
	}
	return n
}

// Ponta a ponta do ciclo editorial com o Postgres real: os "Testes
// obrigatórios" do desenho da fase 2.
func TestEditorialCycleEndToEnd(t *testing.T) {
	a := newEditorialApp(t, nil)
	trabalho := a.topic("Trabalho")
	saude := a.topic("Saúde")
	pid := a.importAndSubmit()
	base := "/api/team/propositions/" + pid
	v1, rev := a.pending(pid)
	content := base + "/versions/" + v1 + "/content"
	approve := base + "/versions/" + v1 + "/approve"

	// Edu grava sem Temas: assume a Posse.
	a.must(a.do("PUT", content, "edu", contentBody("Reduz a jornada de trabalho.", "Jornada menor.", nil, rev)), 200, "edu saves")
	if d := a.detail(pid, "rita"); d.Claim == nil || d.Claim.HolderName != "Edu" || d.CanEdit {
		t.Fatalf("claim after save: %+v", d)
	}
	// Segunda pessoa grava com Posse alheia: 409.
	a.expectErr(a.do("PUT", content, "rita", contentBody("Outro", "Outro", []string{trabalho}, 1)), 409, "claim_taken", "rita with edu's claim")
	// Editor (não Revisor) tenta aprovar: 403.
	a.expectErr(a.do("POST", approve, "edu", ""), 403, "forbidden", "editor approves")
	// Conteúdo incompleto (sem Temas): 409.
	a.expectErr(a.do("POST", approve, "rui", ""), 409, "content_incomplete", "incomplete")
	a.must(a.do("PUT", content, "edu", contentBody("Reduz a jornada de trabalho.", "Jornada menor.", []string{trabalho}, 1)), 200, "edu completes")
	// Gravação com revision velho: 409 e nada muda.
	a.expectErr(a.do("PUT", content, "edu", contentBody("POR CIMA", "POR CIMA", []string{saude}, 1)), 409, "stale_content", "stale")
	if d := a.detail(pid, "edu"); d.PendingContent.Summary != "Reduz a jornada de trabalho." || d.PendingContent.Revision != 2 {
		t.Fatalf("stale write changed content: %+v", d.PendingContent)
	}
	// Posse expirada: a Rita assume ao gravar e vira autora.
	if _, err := a.pool.Exec(context.Background(), `UPDATE queue.claims SET last_activity_at = now() - interval '4 days'`); err != nil {
		t.Fatal(err)
	}
	a.must(a.do("PUT", content, "rita", contentBody("Reduz a jornada de trabalho semanal.", "Jornada menor.", []string{trabalho}, 2)), 200, "rita after expiry")
	if d := a.detail(pid, "edu"); d.Claim == nil || d.Claim.HolderName != "Rita" || len(d.PendingContent.Authors) != 2 {
		t.Fatalf("expired claim not taken: %+v", d)
	}
	// Autora do Resumo tenta aprovar: 403 four_eyes.
	if d := a.detail(pid, "rita"); d.CanApprove || len(d.ApproveBlockers) != 1 || d.ApproveBlockers[0].Code != "four_eyes" {
		t.Errorf("rita blockers: %+v", d)
	}
	a.expectErr(a.do("POST", approve, "rita", ""), 403, "four_eyes", "author approves")

	// Antes da aprovação, o público só vê dados oficiais.
	if body := a.public(); strings.Contains(body, "Reduz a jornada de trabalho semanal") || !strings.Contains(body, `"under_review":true`) {
		t.Fatalf("pending content leaked before approval: %s", body)
	}

	// Rui aprova: quatro olhos, Selo 2, Posse solta.
	if d := a.detail(pid, "rui"); !d.CanApprove {
		t.Fatalf("rui cannot approve: %+v", d)
	}
	a.must(a.do("POST", approve, "rui", ""), 200, "rui approves")
	if d := a.detail(pid, "edu"); d.ReviewState != "ready" || d.Claim != nil || d.ReviewBadge == nil || *d.ReviewBadge != 2 {
		t.Fatalf("after approval: %+v", d)
	}
	body := a.public()
	for _, want := range []string{`"summary":"Reduz a jornada de trabalho semanal."`, `"review_badge":2`, `"topics":["Trabalho"]`, `"pending_version":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("public after approval lacks %s: %s", want, body)
		}
	}

	// Versão nova: conteúdo copiado a confirmar; Proposição volta para
	// em revisão; o público continua com a vigente e ganha o aviso.
	w := a.must(a.do("POST", base+"/versions", "edu", `{"label":"Substitutivo da CCJ","official_text_url":"https://www.camara.leg.br/sub.pdf"}`), 201, "new version")
	if !strings.Contains(w.Body.String(), `"needs_confirmation":true`) {
		t.Fatalf("new version content: %s", w.Body)
	}
	v2, rev2 := a.pending(pid)
	if d := a.detail(pid, "edu"); d.ReviewState != "in_review" {
		t.Fatalf("new version must reopen review: %s", d.ReviewState)
	}
	approve2 := base + "/versions/" + v2 + "/approve"
	// Conteúdo trazido sem confirmar: 409.
	a.expectErr(a.do("POST", approve2, "rui", ""), 409, "content_incomplete", "needs confirmation")
	a.must(a.do("PUT", base+"/versions/"+v2+"/content", "edu", contentBody("SEGREDO-RESUMO-NOVO", "SEGREDO-CURTO-NOVO", []string{trabalho, saude}, rev2)), 200, "edu edits v2")
	// Nada da Versão em análise aparece no público, nem com uma vigente.
	body = a.public()
	if strings.Contains(body, "SEGREDO") || !strings.Contains(body, `"summary":"Reduz a jornada de trabalho semanal."`) ||
		!strings.Contains(body, `"pending_version":{"label":"Substitutivo da CCJ","official_text_url":"https://www.camara.leg.br/sub.pdf"}`) {
		t.Fatalf("public with pending version: %s", body)
	}
	// (authors no público é o Autor oficial da Proposição; a Autoria
	// editorial seria uma lista de ids da equipe.)
	for _, leak := range []string{"needs_confirmation", "revision", a.ids["ana"], a.ids["edu"], a.ids["rita"], a.ids["rui"], "approved_by", "claim"} {
		if strings.Contains(body, leak) {
			t.Errorf("public leaks %q", leak)
		}
	}

	// Admin autor aprova: sem quatro olhos. Antes, solta a Posse do Edu
	// (Auditoria) e confirma o conteúdo trazido (vira autora).
	a.must(a.do("DELETE", base+"/claim", "ana", ""), 204, "admin releases edu's claim")
	_, rev2 = a.pending(pid)
	a.must(a.do("POST", base+"/versions/"+v2+"/content/confirm", "ana", fmt.Sprintf(`{"revision":%d}`, rev2)), 200, "ana confirms")
	w = a.must(a.do("POST", approve2, "ana", ""), 200, "admin self-approves")
	if !strings.Contains(w.Body.String(), `"four_eyes":false`) || !strings.Contains(w.Body.String(), `"review_badge":1`) {
		t.Fatalf("admin approval: %s", w.Body)
	}
	if body := a.public(); !strings.Contains(body, `"review_badge":1`) || !strings.Contains(body, `"summary":"SEGREDO-RESUMO-NOVO"`) {
		t.Errorf("public after admin approval: %s", body)
	}
	// A Fila mostra a Segunda revisão pendente.
	q := a.must(a.do("GET", "/api/team/queue", "edu", ""), 200, "queue").Body.String()
	if !strings.Contains(q, `"reason":"second_review"`) || !strings.Contains(q, v2) {
		t.Fatalf("queue lacks second review: %s", q)
	}
	if q := a.must(a.do("GET", "/api/team/queue?sphere=municipal", "edu", ""), 200, "queue filter").Body.String(); strings.Contains(q, pid) {
		t.Errorf("sphere filter: %s", q)
	}
	second := base + "/versions/" + v2 + "/second-review"
	// O próprio aprovador tenta a segunda: recusado.
	a.expectErr(a.do("POST", second, "ana", ""), 403, "four_eyes", "approver second review")
	// Autores (Edu escreveu a v2; Rita, a v1 copiada) também não.
	a.expectErr(a.do("POST", second, "rita", ""), 403, "four_eyes", "author second review")
	if d := a.detail(pid, "rui"); !d.CanSecondReview {
		t.Errorf("rui should be able to second-review: %+v", d)
	}
	a.must(a.do("POST", second, "rui", ""), 200, "rui second review")
	if body := a.public(); !strings.Contains(body, `"review_badge":2`) {
		t.Errorf("badge after second review: %s", body)
	}
	if q := a.must(a.do("GET", "/api/team/queue", "edu", ""), 200, "queue").Body.String(); strings.Contains(q, "second_review") {
		t.Errorf("second review still queued: %s", q)
	}

	for action, want := range map[string]int{
		"content.update": 4, "content.confirm": 1, "version.create": 1, "review.approve": 2,
		"review.second_review": 1, "claim.release": 1, "topic.create": 2,
	} {
		if got := a.auditCount(action); got != want {
			t.Errorf("audit %s = %d, want %d", action, got, want)
		}
	}
}

// Aprovação e Auditoria andam juntas: se a Auditoria falha, nada da
// aprovação fica (Versão, estado da Proposição, Posse).
func TestApprovalRollsBackWhenAuditFails(t *testing.T) {
	boom := errors.New("auditoria fora do ar")
	a := newEditorialApp(t, func(next recordAudit) recordAudit {
		return func(ctx context.Context, in auditapp.RecordEventInput) error {
			if in.Action == "review.approve" {
				return boom
			}
			return next(ctx, in)
		}
	})
	trabalho := a.topic("Trabalho")
	pid := a.importAndSubmit()
	v1, rev := a.pending(pid)
	base := "/api/team/propositions/" + pid
	a.must(a.do("PUT", base+"/versions/"+v1+"/content", "edu", contentBody("Resumo", "Curto", []string{trabalho}, rev)), 200, "edu saves")

	a.expectErr(a.do("POST", base+"/versions/"+v1+"/approve", "rui", ""), 500, "internal", "approve with failing audit")
	d := a.detail(pid, "edu")
	if d.ReviewState != "in_review" || d.Claim == nil || d.Claim.HolderName != "Edu" || d.ReviewBadge != nil {
		t.Fatalf("approval not rolled back: %+v", d)
	}
	for _, v := range d.Versions {
		if v.ID == v1 && v.State != "pending" {
			t.Errorf("version promoted: %+v", v)
		}
	}
	var approvals int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM review.approvals`).Scan(&approvals); err != nil || approvals != 0 {
		t.Errorf("approvals = %d (%v)", approvals, err)
	}
	if body := a.public(); strings.Contains(body, `"summary":"Resumo"`) {
		t.Errorf("public shows rolled-back content: %s", body)
	}
}

// A conta desativada perde as Posses: quem chega depois edita.
func TestDeactivatedAccountClaimIsIgnored(t *testing.T) {
	a := newEditorialApp(t, nil)
	pid := a.importAndSubmit()
	v1, rev := a.pending(pid)
	content := "/api/team/propositions/" + pid + "/versions/" + v1 + "/content"
	a.must(a.do("PUT", content, "edu", contentBody("Resumo", "", nil, rev)), 200, "edu saves")
	a.must(a.do("POST", "/api/team/users/"+a.ids["edu"]+"/deactivate", "ana", ""), 200, "deactivate edu")
	if d := a.detail(pid, "rita"); d.Claim != nil || !d.CanEdit {
		t.Fatalf("deactivated holder still blocks: %+v", d)
	}
	a.must(a.do("PUT", content, "rita", contentBody("Resumo da Rita", "", nil, rev+1)), 200, "rita saves")
	if d := a.detail(pid, "rita"); d.Claim == nil || d.Claim.HolderName != "Rita" {
		t.Errorf("claim after deactivation: %+v", d.Claim)
	}
}
