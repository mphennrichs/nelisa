package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	auditapp "github.com/mphennrichs/nelisa/api/internal/audit/app"
)

// Ponta a ponta da Correção menor e da Errata com o Postgres real: os
// "Testes obrigatórios" de docs/desenho/fase-2-correcao-e-errata.md.

type snapshotJSON struct {
	Summary      string   `json:"summary"`
	ShortSummary string   `json:"short_summary"`
	TopicIDs     []string `json:"topic_ids"`
}

type erratumJSON struct {
	ID                 string
	State              string
	Said               string
	Correct            string
	Stale              bool
	CanDecideErratum   bool `json:"can_decide_erratum"`
	CanWithdrawErratum bool `json:"can_withdraw_erratum"`
}

type correctionDetail struct {
	ReviewState    string `json:"review_state"`
	CurrentContent *struct {
		Summary  string
		Revision int
		Authors  []string
	} `json:"current_content"`
	Versions []struct {
		ID, State string
	}
	Claim *struct {
		HolderName string `json:"holder_name"`
	}
	CanMinorFix       bool `json:"can_minor_fix"`
	CanProposeErratum bool `json:"can_propose_erratum"`
	MinorFixes        []struct {
		ID               string
		Fields           []string
		Before, After    snapshotJSON
		CreatedBy        string `json:"created_by"`
		CanCheckMinorFix bool   `json:"can_check_minor_fix"`
	} `json:"minor_fixes"`
	PendingErratum *erratumJSON  `json:"pending_erratum"`
	Errata         []erratumJSON `json:"errata"`
	People         map[string]string
}

func (a *editorialApp) correctionDetail(pid, who string) correctionDetail {
	a.t.Helper()
	w := a.must(a.do("GET", "/api/team/propositions/"+pid, who, ""), 200, "detail")
	var d correctionDetail
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		a.t.Fatal(err)
	}
	return d
}

// current devolve a Versão vigente e a revision do conteúdo dela.
func (a *editorialApp) current(pid string) (string, int) {
	a.t.Helper()
	d := a.correctionDetail(pid, "ana")
	for _, v := range d.Versions {
		if v.State == "current" {
			return v.ID, d.CurrentContent.Revision
		}
	}
	a.t.Fatalf("no current version: %+v", d)
	return "", 0
}

// readyProposition leva a PEC 221/2019 até pronta: o Edu escreve, o Rui
// aprova.
func (a *editorialApp) readyProposition(topics ...string) string {
	a.t.Helper()
	pid := a.importAndSubmit()
	v1, rev := a.pending(pid)
	base := "/api/team/propositions/" + pid
	a.must(a.do("PUT", base+"/versions/"+v1+"/content", "edu", contentBody("Reduz a jornada de trablaho.", "Jornada menor.", topics, rev)), 200, "edu writes")
	a.must(a.do("POST", base+"/versions/"+v1+"/approve", "rui", a.revBody(pid)), 200, "rui approves")
	return pid
}

func erratumBody(summary, short string, topics []string, said, correct string, rev int) string {
	b, _ := json.Marshal(map[string]any{
		"summary": summary, "short_summary": short, "topic_ids": topics, "said": said, "correct": correct, "revision": rev,
	})
	return string(b)
}

func (a *editorialApp) queueBody() string {
	a.t.Helper()
	return a.must(a.do("GET", "/api/team/queue", "edu", ""), 200, "queue").Body.String()
}

func TestMinorFixEndToEnd(t *testing.T) {
	a := newEditorialApp(t, nil)
	trabalho := a.topic("Trabalho")
	pid := a.readyProposition(trabalho)
	base := "/api/team/propositions/" + pid
	v1, rev := a.current(pid)
	fix := base + "/versions/" + v1 + "/minor-fix"

	if d := a.correctionDetail(pid, "rita"); !d.CanMinorFix || !d.CanProposeErratum {
		t.Fatalf("ready proposition must accept corrections: %+v", d)
	}
	// Revision velha: 409, nada muda.
	a.expectErr(a.do("PUT", fix, "rita", contentBody("Reduz a jornada de trabalho.", "Jornada menor.", []string{trabalho}, rev-1)), 409, "stale_content", "stale minor fix")
	// Sem mudança nenhuma: não é correção.
	a.expectErr(a.do("PUT", fix, "rita", contentBody("Reduz a jornada de trablaho.", "Jornada menor.", []string{trabalho}, rev)), 409, "no_changes", "no-op minor fix")
	// Correção que esvaziaria o que o Leitor já vê: recusada.
	a.expectErr(a.do("PUT", fix, "rita", contentBody("Reduz a jornada de trabalho.", "", []string{trabalho}, rev)), 400, "published_content_incomplete", "emptying fix")
	if d := a.correctionDetail(pid, "rita"); d.Claim != nil {
		t.Fatalf("refused fixes must not take the claim: %+v", d.Claim)
	}

	// A Rita (Revisora) corrige a digitação: entra na hora, segue pronta.
	w := a.must(a.do("PUT", fix, "rita", contentBody("Reduz a jornada de trabalho.", "Jornada menor.", []string{trabalho}, rev)), 200, "rita fixes")
	if !strings.Contains(w.Body.String(), `"fields":["summary"]`) {
		t.Fatalf("minor fix fields: %s", w.Body)
	}
	d := a.correctionDetail(pid, "rui")
	if d.ReviewState != "ready" || d.CurrentContent.Summary != "Reduz a jornada de trabalho." || d.Claim == nil || d.Claim.HolderName != "Rita" {
		t.Fatalf("after minor fix: %+v", d)
	}
	if len(d.CurrentContent.Authors) != 2 || !contains(d.CurrentContent.Authors, a.ids["rita"]) {
		t.Errorf("fixer must join authors: %v", d.CurrentContent.Authors)
	}
	if len(d.MinorFixes) != 1 || d.MinorFixes[0].Before.Summary != "Reduz a jornada de trablaho." ||
		d.MinorFixes[0].After.Summary != "Reduz a jornada de trabalho." || !d.MinorFixes[0].CanCheckMinorFix ||
		d.People[d.MinorFixes[0].CreatedBy] != "Rita" {
		t.Fatalf("minor fix to check: %+v", d.MinorFixes)
	}
	fixID := d.MinorFixes[0].ID
	// O Leitor vê o texto corrigido, sem aviso nenhum.
	body := a.public()
	if !strings.Contains(body, `"summary":"Reduz a jornada de trabalho."`) || strings.Contains(body, "trablaho") ||
		strings.Contains(body, "minor") || strings.Contains(body, a.ids["rita"]) || !strings.Contains(body, `"errata":[]`) {
		t.Fatalf("public after minor fix: %s", body)
	}
	// Na Fila, como correção a conferir.
	if q := a.queueBody(); !strings.Contains(q, `"reason":"minor_fix_check"`) || !strings.Contains(q, pid) {
		t.Fatalf("queue lacks minor fix check: %s", q)
	}
	// Posse alheia: o Edu leva 409.
	_, rev = a.current(pid)
	a.expectErr(a.do("PUT", fix, "edu", contentBody("Reduz a jornada de trabalho!", "Jornada menor.", []string{trabalho}, rev)), 409, "claim_taken", "edu with rita's claim")
	if d := a.correctionDetail(pid, "edu"); d.CanMinorFix {
		t.Errorf("can_minor_fix with someone else's claim")
	}

	check := base + "/minor-fixes/" + fixID + "/check"
	// Quem corrigiu tenta conferir: 403. O Editor também não confere.
	if d := a.correctionDetail(pid, "rita"); d.MinorFixes[0].CanCheckMinorFix {
		t.Errorf("fixer can check own fix")
	}
	a.expectErr(a.do("POST", check, "rita", ""), 403, "four_eyes", "fixer checks")
	a.expectErr(a.do("POST", check, "edu", ""), 403, "forbidden", "editor checks")
	a.expectErr(a.do("POST", "/api/team/propositions/"+v1+"/minor-fixes/"+fixID+"/check", "rui", ""), 404, "minor_fix_not_found", "check under another proposition")
	// Outro Revisor confere: sai da Fila.
	a.must(a.do("POST", check, "rui", ""), 200, "rui checks")
	a.expectErr(a.do("POST", check, "ana", ""), 409, "minor_fix_checked", "check twice")
	if q := a.queueBody(); strings.Contains(q, "minor_fix_check") {
		t.Errorf("checked fix still queued: %s", q)
	}
	if d := a.correctionDetail(pid, "rui"); len(d.MinorFixes) != 0 {
		t.Errorf("checked fix still listed: %+v", d.MinorFixes)
	}

	// Versão nova em análise: a Correção menor só vale na vigente de uma
	// Proposição pronta.
	a.must(a.do("DELETE", base+"/claim", "rita", ""), 204, "rita releases")
	a.must(a.do("POST", base+"/versions", "edu", `{"label":"Substitutivo","official_text_url":"https://sapl.camara.goiania.go.gov.br/sub.pdf"}`), 201, "new version")
	v2, rev2 := a.pending(pid)
	_, rev = a.current(pid)
	a.expectErr(a.do("PUT", base+"/versions/"+v2+"/minor-fix", "edu", contentBody("X", "Y", []string{trabalho}, rev2)), 409, "version_not_current", "fix on pending version")
	a.expectErr(a.do("PUT", fix, "edu", contentBody("Reduz a jornada!", "Jornada menor.", []string{trabalho}, rev)), 409, "not_ready", "fix on current while in review")
	if d := a.correctionDetail(pid, "edu"); d.CanMinorFix || d.CanProposeErratum {
		t.Errorf("in review must not accept corrections: %+v", d)
	}
	a.must(a.do("POST", base+"/versions/"+v2+"/content/confirm", "edu", fmt.Sprintf(`{"revision":%d}`, rev2)), 200, "edu confirms")
	a.must(a.do("POST", base+"/versions/"+v2+"/approve", "rui", a.revBody(pid)), 200, "rui approves v2")
	// A antiga, agora substituída: recusada.
	a.expectErr(a.do("PUT", fix, "edu", contentBody("Reduz a jornada!", "Jornada menor.", []string{trabalho}, rev)), 409, "version_not_current", "fix on superseded version")

	for action, want := range map[string]int{"content.minor_fix": 1, "review.minor_fix_check": 1} {
		if got := a.auditCount(action); got != want {
			t.Errorf("audit %s = %d, want %d", action, got, want)
		}
	}
}

func TestErratumEndToEnd(t *testing.T) {
	a := newEditorialApp(t, nil)
	trabalho := a.topic("Trabalho")
	saude := a.topic("Saúde")
	pid := a.readyProposition(trabalho)
	base := "/api/team/propositions/" + pid
	v1, rev := a.current(pid)
	errata := base + "/versions/" + v1 + "/errata"

	// A Ana (Admin) propõe: não precisa de Posse.
	w := a.must(a.do("POST", errata, "ana", erratumBody("Aumenta a jornada de trabalho.", "Jornada maior.", []string{trabalho, saude},
		"SEGREDO-DIZIA a jornada diminui", "SEGREDO-CORRETO a jornada aumenta", rev)), 201, "ana proposes")
	var e1 struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &e1)
	// Enquanto pendente, nada muda para o Leitor.
	body := a.public()
	if strings.Contains(body, "SEGREDO") || strings.Contains(body, "Aumenta") || !strings.Contains(body, `"summary":"Reduz a jornada de trablaho."`) ||
		!strings.Contains(body, `"errata":[]`) {
		t.Fatalf("pending erratum leaked: %s", body)
	}
	if d := a.correctionDetail(pid, "edu"); d.ReviewState != "ready" || d.CurrentContent.Revision != rev {
		t.Fatalf("pending erratum changed content: %+v", d)
	}
	if q := a.queueBody(); !strings.Contains(q, `"reason":"erratum"`) {
		t.Fatalf("queue lacks erratum: %s", q)
	}
	// Segunda Errata pendente na mesma Versão: 409. Correção menor também.
	a.expectErr(a.do("POST", errata, "edu", erratumBody("Outro.", "Outro.", []string{trabalho}, "a", "b", rev)), 409, "erratum_pending", "second erratum")
	a.expectErr(a.do("PUT", base+"/versions/"+v1+"/minor-fix", "edu", contentBody("Reduz a jornada de trabalho.", "Jornada menor.", []string{trabalho}, rev)), 409, "erratum_pending", "minor fix with pending erratum")
	if d := a.correctionDetail(pid, "edu"); d.CanMinorFix || d.CanProposeErratum || d.PendingErratum == nil || d.PendingErratum.CanDecideErratum {
		t.Fatalf("detail with pending erratum (editor): %+v", d)
	}

	approve := errata + "/" + e1.ID + "/approve"
	revBody := fmt.Sprintf(`{"revision":%d}`, rev)
	// Quem propôs tenta aprovar, mesmo Admin: 403. Editor: 403.
	if d := a.correctionDetail(pid, "ana"); d.PendingErratum.CanDecideErratum || !d.PendingErratum.CanWithdrawErratum {
		t.Errorf("proposer decisions: %+v", d.PendingErratum)
	}
	a.expectErr(a.do("POST", approve, "ana", revBody), 403, "four_eyes", "admin proposer approves")
	a.expectErr(a.do("POST", errata+"/"+e1.ID+"/reject", "ana", `{"reason":"não"}`), 403, "four_eyes", "admin proposer rejects")
	a.expectErr(a.do("POST", approve, "edu", revBody), 403, "forbidden", "editor approves")
	a.expectErr(a.do("POST", approve, "rui", fmt.Sprintf(`{"revision":%d}`, rev-1)), 409, "stale_content", "approve with old revision")
	a.expectErr(a.do("POST", errata+"/"+e1.ID+"/withdraw", "rita", ""), 403, "forbidden", "someone else withdraws")
	if d := a.correctionDetail(pid, "rui"); !d.PendingErratum.CanDecideErratum || d.PendingErratum.Stale {
		t.Fatalf("rui decisions: %+v", d.PendingErratum)
	}
	// Outro Revisor aprova: conteúdo novo e a nota no público.
	a.must(a.do("POST", approve, "rui", revBody), 200, "rui approves erratum")
	d := a.correctionDetail(pid, "rui")
	if d.CurrentContent.Summary != "Aumenta a jornada de trabalho." || d.CurrentContent.Revision != rev+1 ||
		!contains(d.CurrentContent.Authors, a.ids["ana"]) || contains(d.CurrentContent.Authors, a.ids["rui"]) || d.ReviewState != "ready" {
		t.Fatalf("content after erratum approval: %+v", d.CurrentContent)
	}
	if d.PendingErratum != nil || len(d.Errata) != 1 || d.Errata[0].State != "approved" {
		t.Fatalf("erratum history: %+v", d)
	}
	body = a.public()
	for _, want := range []string{`"summary":"Aumenta a jornada de trabalho."`, `"topics":["Trabalho","Saúde"]`,
		`"said":"SEGREDO-DIZIA a jornada diminui","correct":"SEGREDO-CORRETO a jornada aumenta"`, `"approved_at":"`} {
		if !strings.Contains(body, want) {
			t.Errorf("public after erratum lacks %s: %s", want, body)
		}
	}

	// Recusada, retirada e pendente nunca saem; nem quem propôs ou decidiu.
	_, rev = a.current(pid)
	w = a.must(a.do("POST", errata, "edu", erratumBody("RECUSADA-RESUMO", "RECUSADA-CURTO", []string{trabalho}, "RECUSADA-DIZIA", "RECUSADA-CORRETO", rev)), 201, "edu proposes")
	var e2 struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &e2)
	a.expectErr(a.do("POST", errata+"/"+e2.ID+"/reject", "rita", `{"reason":"  "}`), 400, "invalid_reason", "reject without reason")
	a.must(a.do("POST", errata+"/"+e2.ID+"/reject", "rita", `{"reason":"MOTIVO-INTERNO"}`), 200, "rita rejects")
	a.expectErr(a.do("POST", errata+"/"+e2.ID+"/approve", "rui", fmt.Sprintf(`{"revision":%d}`, rev)), 409, "erratum_not_pending", "approve rejected")
	w = a.must(a.do("POST", errata, "edu", erratumBody("RETIRADA-RESUMO", "RETIRADA-CURTO", []string{trabalho}, "RETIRADA-DIZIA", "RETIRADA-CORRETO", rev)), 201, "edu proposes again")
	var e3 struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &e3)
	a.must(a.do("POST", errata+"/"+e3.ID+"/withdraw", "edu", ""), 200, "edu withdraws")
	w = a.must(a.do("POST", errata, "edu", erratumBody("PENDENTE-RESUMO", "PENDENTE-CURTO", []string{trabalho}, "PENDENTE-DIZIA", "PENDENTE-CORRETO", rev)), 201, "edu proposes a third")
	var e4 struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &e4)
	body = a.public()
	for _, leak := range []string{"RECUSADA", "RETIRADA", "PENDENTE", "MOTIVO-INTERNO", "reason", "decided", "created_by", "proposed",
		`"state"`, a.ids["ana"], a.ids["edu"], a.ids["rita"], a.ids["rui"], "Rita", "Rui"} {
		if strings.Contains(body, leak) {
			t.Errorf("public leaks %q: %s", leak, body)
		}
	}
	if n := strings.Count(body, `"said":`); n != 1 {
		t.Errorf("public errata count = %d", n)
	}

	// O conteúdo base mudou depois da proposta: aprovar dá 409 e nada muda.
	if _, err := a.pool.Exec(context.Background(), `UPDATE proposition.version_contents SET revision = revision + 1 WHERE version_id = $1`, v1); err != nil {
		t.Fatal(err)
	}
	if d := a.correctionDetail(pid, "rui"); d.PendingErratum == nil || !d.PendingErratum.Stale {
		t.Errorf("stale erratum not flagged: %+v", d.PendingErratum)
	}
	a.expectErr(a.do("POST", errata+"/"+e4.ID+"/approve", "rui", fmt.Sprintf(`{"revision":%d}`, rev)), 409, "stale_content", "approve over changed base")
	if d := a.correctionDetail(pid, "rui"); d.CurrentContent.Summary != "Aumenta a jornada de trabalho." || d.PendingErratum == nil {
		t.Fatalf("stale approval changed something: %+v", d)
	}
	a.must(a.do("POST", errata+"/"+e4.ID+"/withdraw", "edu", ""), 200, "edu withdraws the stale one")

	// Versão nova vira vigente: as Erratas da anterior saem com o conteúdo
	// dela.
	a.must(a.do("POST", base+"/versions", "edu", `{"label":"Substitutivo","official_text_url":"https://sapl.camara.goiania.go.gov.br/sub.pdf"}`), 201, "new version")
	v2, rev2 := a.pending(pid)
	a.must(a.do("POST", base+"/versions/"+v2+"/content/confirm", "edu", fmt.Sprintf(`{"revision":%d}`, rev2)), 200, "edu confirms")
	a.must(a.do("POST", base+"/versions/"+v2+"/approve", "rui", a.revBody(pid)), 200, "rui approves v2")
	if body := a.public(); strings.Contains(body, "SEGREDO") || !strings.Contains(body, `"errata":[]`) {
		t.Errorf("errata of superseded version still public: %s", body)
	}

	for action, want := range map[string]int{
		"review.erratum_propose": 4, "review.erratum_approve": 1, "review.erratum_reject": 1, "review.erratum_withdraw": 2,
	} {
		if got := a.auditCount(action); got != want {
			t.Errorf("audit %s = %d, want %d", action, got, want)
		}
	}
}

// Correção, conferência e decisões da Errata andam com a Auditoria: se ela
// falha, nada fica.
func TestCorrectionsRollBackWhenAuditFails(t *testing.T) {
	var mu sync.Mutex
	failing := map[string]bool{}
	setFailing := func(action string, on bool) { mu.Lock(); defer mu.Unlock(); failing[action] = on }
	boom := errors.New("auditoria fora do ar")
	a := newEditorialApp(t, func(next recordAudit) recordAudit {
		return func(ctx context.Context, in auditapp.RecordEventInput) error {
			mu.Lock()
			fail := failing[in.Action]
			mu.Unlock()
			if fail {
				return boom
			}
			return next(ctx, in)
		}
	})
	trabalho := a.topic("Trabalho")
	pid := a.readyProposition(trabalho)
	base := "/api/team/propositions/" + pid
	v1, rev := a.current(pid)
	fix := base + "/versions/" + v1 + "/minor-fix"
	count := func(table string) int {
		var n int
		if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM review.`+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	setFailing("content.minor_fix", true)
	a.expectErr(a.do("PUT", fix, "rita", contentBody("Reduz a jornada de trabalho.", "Jornada menor.", []string{trabalho}, rev)), 500, "internal", "minor fix with failing audit")
	if d := a.correctionDetail(pid, "rui"); d.CurrentContent.Summary != "Reduz a jornada de trablaho." || d.CurrentContent.Revision != rev || d.Claim != nil || count("minor_fixes") != 0 {
		t.Fatalf("minor fix not rolled back: %+v", d)
	}
	setFailing("content.minor_fix", false)
	a.must(a.do("PUT", fix, "rita", contentBody("Reduz a jornada de trabalho.", "Jornada menor.", []string{trabalho}, rev)), 200, "rita fixes")
	fixID := a.correctionDetail(pid, "rui").MinorFixes[0].ID

	setFailing("review.minor_fix_check", true)
	a.expectErr(a.do("POST", base+"/minor-fixes/"+fixID+"/check", "rui", ""), 500, "internal", "check with failing audit")
	if d := a.correctionDetail(pid, "rui"); len(d.MinorFixes) != 1 {
		t.Fatalf("check not rolled back: %+v", d.MinorFixes)
	}
	a.must(a.do("DELETE", base+"/claim", "rita", ""), 204, "rita releases")

	_, rev = a.current(pid)
	errata := base + "/versions/" + v1 + "/errata"
	setFailing("review.erratum_propose", true)
	a.expectErr(a.do("POST", errata, "edu", erratumBody("Novo.", "Novo.", []string{trabalho}, "a", "b", rev)), 500, "internal", "propose with failing audit")
	if count("errata") != 0 {
		t.Fatal("proposal not rolled back")
	}
	setFailing("review.erratum_propose", false)
	w := a.must(a.do("POST", errata, "edu", erratumBody("Novo resumo.", "Novo curto.", []string{trabalho}, "a", "b", rev)), 201, "edu proposes")
	var e struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	for _, step := range []struct{ action, path, who, body string }{
		{"review.erratum_approve", "/approve", "rui", fmt.Sprintf(`{"revision":%d}`, rev)},
		{"review.erratum_reject", "/reject", "rui", `{"reason":"não muda o sentido"}`},
		{"review.erratum_withdraw", "/withdraw", "edu", ""},
	} {
		setFailing(step.action, true)
		a.expectErr(a.do("POST", errata+"/"+e.ID+step.path, step.who, step.body), 500, "internal", step.action+" with failing audit")
		d := a.correctionDetail(pid, "rui")
		if d.PendingErratum == nil || d.PendingErratum.State != "pending" || d.CurrentContent.Summary != "Reduz a jornada de trabalho." || d.CurrentContent.Revision != rev {
			t.Fatalf("%s not rolled back: %+v", step.action, d)
		}
		setFailing(step.action, false)
	}
	if body := a.public(); strings.Contains(body, "Novo resumo") {
		t.Errorf("rolled back erratum is public: %s", body)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
