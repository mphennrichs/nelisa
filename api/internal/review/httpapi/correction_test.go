package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/review/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

const (
	minorFix = "/api/team/propositions/p1/versions/v1/minor-fix"
	errata   = "/api/team/propositions/p1/versions/v1/errata"
)

func readyWorld() *fakes.World {
	w := fakes.NewWorld()
	w.PutProposition(fakes.Proposition{ID: "p1", Name: "PEC 1/2020", State: "ready"})
	w.PutVersion(fakes.Version{
		ID: "v1", PropositionID: "p1", Seq: 1, State: "current", Revision: 2, Authors: []string{"ed"},
		Content: domain.Snapshot{Summary: "Resumo", ShortSummary: "Curto", TopicIDs: []string{"t1"}},
	})
	w.Names["rui"] = "Rui"
	return w
}

func contentJSON(summary string, rev int, extra string) string {
	return fmt.Sprintf(`{"summary":%q,"short_summary":"Curto","topic_ids":["t1"],"revision":%d%s}`, summary, rev, extra)
}

func TestMinorFixHTTP(t *testing.T) {
	w := readyWorld()
	h := newServer(w)
	for _, tc := range []struct {
		name, actor, body string
		status            int
		code              string
	}{
		{"anonymous", "", contentJSON("Resumo.", 2, ""), http.StatusUnauthorized, "unauthenticated"},
		{"no revision", "ed:editor", `{"summary":"x"}`, http.StatusBadRequest, "revision_required"},
		{"stale", "ed:editor", contentJSON("Resumo.", 1, ""), http.StatusConflict, "stale_content"},
		{"incomplete", "ed:editor", `{"summary":"x","revision":2}`, http.StatusBadRequest, "published_content_incomplete"},
		{"no change", "ed:editor", contentJSON("Resumo", 2, ""), http.StatusConflict, "no_changes"},
	} {
		rec := doMethod(h, "PUT", minorFix, tc.actor, tc.body)
		if rec.Code != tc.status || errCode(t, rec) != tc.code {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
	w.SetClaim("p1", "rui")
	rec := doMethod(h, "PUT", minorFix, "ed:editor", contentJSON("Resumo.", 2, ""))
	if rec.Code != http.StatusConflict || errCode(t, rec) != "claim_taken" || !strings.Contains(rec.Body.String(), "Rui está com a posse") {
		t.Fatalf("claim taken: %d %s", rec.Code, rec.Body)
	}
	w.SetClaim("p1", "ed")
	rec = doMethod(h, "PUT", minorFix, "ed:editor", contentJSON("Resumo.", 2, ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"fields":["summary"]`) || !strings.Contains(rec.Body.String(), `"before":{"summary":"Resumo"`) {
		t.Fatalf("minor fix: %d %s", rec.Code, rec.Body)
	}
	var f struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &f)
	check := "/api/team/propositions/p1/minor-fixes/" + f.ID + "/check"
	if rec := doBody(h, check, "ed:editor", ""); rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden" {
		t.Errorf("editor checks: %d %s", rec.Code, rec.Body)
	}
	if rec := doBody(h, "/api/team/propositions/p1/minor-fixes/nope/check", "rui:reviewer", ""); rec.Code != http.StatusNotFound || errCode(t, rec) != "minor_fix_not_found" {
		t.Errorf("unknown fix: %d %s", rec.Code, rec.Body)
	}
	if rec := doBody(h, check, "rui:reviewer", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"checked_by":"rui"`) {
		t.Errorf("check: %d %s", rec.Code, rec.Body)
	}
}

func TestErratumHTTP(t *testing.T) {
	w := readyWorld()
	h := newServer(w)
	if rec := doBody(h, errata, "ed:editor", contentJSON("Outro", 2, `,"said":"","correct":"y"`)); rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_erratum_note" {
		t.Fatalf("empty note: %d %s", rec.Code, rec.Body)
	}
	rec := doBody(h, errata, "ana:admin", contentJSON("Outro", 2, `,"said":"x","correct":"y"`))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"state":"pending"`) {
		t.Fatalf("propose: %d %s", rec.Code, rec.Body)
	}
	var e struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	if rec := doBody(h, errata, "ed:editor", contentJSON("Mais", 2, `,"said":"x","correct":"y"`)); rec.Code != http.StatusConflict || errCode(t, rec) != "erratum_pending" {
		t.Errorf("second pending: %d %s", rec.Code, rec.Body)
	}
	if rec := doMethod(h, "PUT", minorFix, "ed:editor", contentJSON("Resumo.", 2, "")); rec.Code != http.StatusConflict || errCode(t, rec) != "erratum_pending" {
		t.Errorf("fix with pending erratum: %d %s", rec.Code, rec.Body)
	}
	one := errata + "/" + e.ID
	for _, tc := range []struct {
		path, actor, body string
		status            int
		code              string
	}{
		{one + "/approve", "ana:admin", `{"revision":2}`, http.StatusForbidden, "four_eyes"},
		{one + "/approve", "ed:editor", `{"revision":2}`, http.StatusForbidden, "forbidden"},
		{one + "/approve", "rui:reviewer", `{}`, http.StatusBadRequest, "revision_required"},
		{one + "/approve", "rui:reviewer", `{"revision":1}`, http.StatusConflict, "stale_content"},
		{one + "/reject", "rui:reviewer", `{"reason":""}`, http.StatusBadRequest, "invalid_reason"},
		{one + "/withdraw", "rui:reviewer", `{}`, http.StatusForbidden, "forbidden"},
		{errata + "/nope/approve", "rui:reviewer", `{"revision":2}`, http.StatusNotFound, "erratum_not_found"},
	} {
		rec := doBody(h, tc.path, tc.actor, tc.body)
		if rec.Code != tc.status || errCode(t, rec) != tc.code {
			t.Errorf("%s by %s: %d %s", tc.path, tc.actor, rec.Code, rec.Body)
		}
	}
	rec = doBody(h, one+"/approve", "rui:reviewer", `{"revision":2}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"state":"approved"`) || !strings.Contains(rec.Body.String(), `"decided_by":"rui"`) {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	if rec := doBody(h, one+"/withdraw", "ana:admin", ""); rec.Code != http.StatusConflict || errCode(t, rec) != "erratum_not_pending" {
		t.Errorf("withdraw approved: %d %s", rec.Code, rec.Body)
	}
}
