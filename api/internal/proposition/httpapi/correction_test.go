package httpapi_test

import (
	"strings"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

func TestPublicDetailCarriesErrataNotesOnly(t *testing.T) {
	s := newServer(t)
	s.seedEditorial(t)
	s.w.Errata = []domain.PublicErratum{{VersionID: vCur, ApprovedAt: t0, Said: "dizia que reduz", Correct: "o correto é que aumenta"}}
	body := s.do("GET", "/api/public/propositions/pec-221-2019", "").Body.String()
	want := `"errata":[{"approved_at":"` + t0.UTC().Format("2006-01-02T15:04:05Z07:00") + `","said":"dizia que reduz","correct":"o correto é que aumenta"}]`
	if !strings.Contains(body, want) {
		t.Fatalf("missing %s in %s", want, body)
	}
	for _, leak := range []string{"version_id", vCur, "decided", "created_by", "reason", "state"} {
		if strings.Contains(body, leak) {
			t.Errorf("public errata leaks %q: %s", leak, body)
		}
	}
	// A lista não traz as notas.
	if list := s.do("GET", "/api/public/propositions", "").Body.String(); strings.Contains(list, "errata") {
		t.Errorf("list carries errata: %s", list)
	}
}

func TestTeamDetailCarriesCorrectionDecisions(t *testing.T) {
	s := newServer(t)
	s.seedEditorial(t)
	s.w.Statuses[pid] = app.ReviewStatus{
		CanMinorFix: true, CanProposeErratum: false,
		MinorFixes: []app.MinorFixInfo{{
			ID: "f1", VersionID: vCur, Fields: []string{"summary"}, CreatedBy: "ana", CreatedAt: t0, CanCheck: true,
			Before: app.ContentSnapshot{Summary: "Antes"}, After: app.ContentSnapshot{Summary: "Depois", TopicIDs: []string{"t-trab"}},
		}},
		PendingErratum: &app.ErratumInfo{ID: "e1", VersionID: vCur, Said: "x", Correct: "y", State: "pending", CreatedBy: "bia", CreatedAt: t0, CanWithdraw: true, Stale: true},
		Errata:         []app.ErratumInfo{{ID: "e0", State: "rejected", CreatedBy: "ana", DecidedBy: "bia", DecidedAt: &t0, Reason: "não muda"}},
	}
	body := s.do("GET", "/api/team/propositions/"+pid, "bia:editor").Body.String()
	for _, want := range []string{
		`"can_minor_fix":true`, `"can_propose_erratum":false`,
		`"minor_fixes":[{"id":"f1"`, `"before":{"summary":"Antes","short_summary":"","topic_ids":[]}`, `"can_check_minor_fix":true`,
		`"pending_erratum":{"id":"e1"`, `"stale":true`, `"can_decide_erratum":false`, `"can_withdraw_erratum":true`,
		`"errata":[{"id":"e0"`, `"reason":"não muda"`, `"Ana Editora"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
}
