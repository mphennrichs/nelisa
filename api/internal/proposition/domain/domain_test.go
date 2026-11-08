package domain_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func mk(t *testing.T, origin domain.Chamber, state domain.ReviewState) domain.Proposition {
	t.Helper()
	p, err := domain.NewProposition("p1", "pec-221-2019", "pec", 221, 2019, origin, domain.OfficialData{
		OfficialSummary: "  Altera a jornada  ", Status: "Em tramitação", Authors: []string{"Dep. Fulana", "  "},
	}, t0)
	if err != nil {
		t.Fatal(err)
	}
	p.ReviewState = state
	return p
}

func TestNewPropositionNormalizesAndStartsAsDraft(t *testing.T) {
	p := mk(t, domain.ChamberCamara, domain.ReviewDraft)
	if p.Name() != "PEC 221/2019" || p.Kind != "PEC" || p.Sphere != domain.SphereFederal || p.ReviewState != domain.ReviewDraft {
		t.Errorf("got %+v", p)
	}
	if p.OfficialSummary != "Altera a jornada" || len(p.Authors) != 1 {
		t.Errorf("official data not cleaned: %+v", p.OfficialData)
	}
	for _, c := range []struct {
		kind         string
		number, year int
		origin       domain.Chamber
		want         error
	}{
		{"P L", 1, 2020, domain.ChamberCamara, domain.ErrInvalidKind},
		{"PL", 0, 2020, domain.ChamberCamara, domain.ErrInvalidNumber},
		{"PL", 1, 20, domain.ChamberCamara, domain.ErrInvalidYear},
		{"PL", 1, 2020, "alesp", domain.ErrInvalidChamber},
	} {
		if _, err := domain.NewProposition("x", "x", c.kind, c.number, c.year, c.origin, domain.OfficialData{}, t0); !errors.Is(err, c.want) {
			t.Errorf("%+v: err = %v", c, err)
		}
	}
}

func TestParseOfficialName(t *testing.T) {
	cases := map[string]string{"PEC 221/2019": "PEC 221 2019", "pl 1234 / 2026": "PL 1234 2026", "PLP nº 5/2021": "PLP 5 2021"}
	for in, want := range cases {
		k, n, y, err := domain.ParseOfficialName(in)
		if got := k + " " + strconv.Itoa(n) + " " + strconv.Itoa(y); err != nil || got != want {
			t.Errorf("%q = %q (%v), want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "PEC", "PEC 221", "221/2019", "PEC 221/19", "<script> 1/2020"} {
		if _, _, _, err := domain.ParseOfficialName(bad); !errors.Is(err, domain.ErrInvalidName) {
			t.Errorf("%q should be invalid, err = %v", bad, err)
		}
	}
}

func TestSlugs(t *testing.T) {
	if s := domain.Slug("PEC", 221, 2019); s != "pec-221-2019" {
		t.Errorf("slug = %q", s)
	}
	if s := domain.SlugWithChamber("PEC", 10, 2015, domain.ChamberSenado); s != "pec-10-2015-senado" {
		t.Errorf("slug = %q", s)
	}
}

func TestSubmitForReviewOnlyFromDraft(t *testing.T) {
	p := mk(t, domain.ChamberCamara, domain.ReviewDraft)
	if err := p.SubmitForReview(t0.Add(time.Hour)); err != nil || p.ReviewState != domain.ReviewInReview || !p.UpdatedAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("draft → in_review: %v %+v", err, p)
	}
	for _, st := range []domain.ReviewState{domain.ReviewInReview, domain.ReviewReady} {
		p := mk(t, domain.ChamberCamara, st)
		if err := p.SubmitForReview(t0); !errors.Is(err, domain.ErrInvalidTransition) || p.ReviewState != st {
			t.Errorf("from %s: err = %v, state = %s", st, err, p.ReviewState)
		}
	}
}

func TestCleanDropsHostileValues(t *testing.T) {
	d := domain.OfficialData{
		OfficialTextURL: "javascript:alert(1)", OfficialSummary: strings.Repeat("a", 20_000), Status: "Declarada Prejudicada",
	}.Clean()
	if d.OfficialTextURL != "" || len([]rune(d.OfficialSummary)) != 10_000 || !d.Superseded {
		t.Errorf("got url=%q len=%d superseded=%v", d.OfficialTextURL, len(d.OfficialSummary), d.Superseded)
	}
}

func ptr(t time.Time) *time.Time { return &t }

func TestApplySyncRules(t *testing.T) {
	p := mk(t, domain.ChamberCamara, domain.ReviewInReview)
	p.StatusAt = ptr(t0)

	// A Casa revisora com evento mais novo dita a situação, mas não a ementa.
	p.ApplySync(domain.SyncUpdate{
		Chamber: domain.ChamberSenado, LatestEventAt: ptr(t0.Add(48 * time.Hour)),
		Data: domain.OfficialData{OfficialSummary: "outra ementa", Status: "Aguardando votação no Senado"},
	}, t0.Add(time.Hour))
	if p.OfficialSummary != "Altera a jornada" || p.Status != "Aguardando votação no Senado" || !p.StatusAt.Equal(t0.Add(48*time.Hour)) {
		t.Errorf("after senado sync: %+v", p)
	}
	// A iniciadora, com evento mais velho, atualiza a identificação mas não
	// a situação.
	p.ApplySync(domain.SyncUpdate{
		Chamber: domain.ChamberCamara, LatestEventAt: ptr(t0.Add(time.Hour)), ReportsAttachment: true,
		Data: domain.OfficialData{OfficialSummary: "ementa nova", Status: "Remetida ao Senado", AttachedToName: "PEC 32/2015"},
	}, t0.Add(2*time.Hour))
	if p.OfficialSummary != "ementa nova" || p.Status != "Aguardando votação no Senado" || p.AttachedToName != "PEC 32/2015" {
		t.Errorf("after camara sync: %+v", p)
	}
	if p.ReviewState != domain.ReviewInReview {
		t.Errorf("sync must never change review state, got %s", p.ReviewState)
	}

	p.AttachTo("principal")
	// Fonte que não informa Apensação (Senado): o vazio não desapensa.
	p.ApplySync(domain.SyncUpdate{Chamber: domain.ChamberSenado, Data: domain.OfficialData{}}, t0)
	if p.AttachedToID == nil || p.AttachedToName != "PEC 32/2015" {
		t.Errorf("senado must not clear attachment: %+v", p)
	}
	// Desapensação informada pela Câmara.
	p.ApplySync(domain.SyncUpdate{Chamber: domain.ChamberCamara, ReportsAttachment: true, Data: domain.OfficialData{}}, t0)
	if p.AttachedToID != nil || p.AttachedToName != "" {
		t.Errorf("camara \"\" must clear attachment: %+v", p)
	}
}

func TestSourceLinkStates(t *testing.T) {
	l, err := domain.NewSuggestedLink("l1", "p1", domain.ChamberSenado, "123", "PEC 221/2019", "javascript:x", t0)
	if err != nil || l.State != domain.LinkSuggested || l.SourceURL != "" || l.ConfirmedAt != nil {
		t.Fatalf("suggested: %v %+v", err, l)
	}
	if err := l.Confirm("ed", t0); err != nil || l.State != domain.LinkConfirmed || l.ConfirmedBy != "ed" {
		t.Fatalf("confirm: %v %+v", err, l)
	}
	if err := l.Confirm("ed", t0); !errors.Is(err, domain.ErrLinkNotSuggested) {
		t.Errorf("confirm twice: %v", err)
	}
	if err := l.CanReject(); !errors.Is(err, domain.ErrLinkNotSuggested) {
		t.Errorf("reject confirmed: %v", err)
	}
	for _, bad := range []string{"", "a/b", "x y", strings.Repeat("9", 65)} {
		if _, err := domain.NewSuggestedLink("l", "p", domain.ChamberCamara, bad, "", "", t0); !errors.Is(err, domain.ErrInvalidExternal) {
			t.Errorf("external %q: %v", bad, err)
		}
	}
	l.SyncFailed(strings.Repeat("e", 900))
	if len(l.LastSyncError) != 500 {
		t.Errorf("sync error not truncated: %d", len(l.LastSyncError))
	}
	l.SyncSucceeded(t0)
	if l.LastSyncError != "" || l.LastSyncedAt == nil {
		t.Errorf("sync success: %+v", l)
	}
}

func TestPublicViewHidesDraftsAndInternalData(t *testing.T) {
	draft := mk(t, domain.ChamberCamara, domain.ReviewDraft)
	if _, ok := domain.PublicView(draft, nil, nil, nil, domain.Editorial{}); ok {
		t.Fatal("draft must not be public")
	}
	p := mk(t, domain.ChamberCamara, domain.ReviewInReview)
	confirmed, _ := domain.NewConfirmedLink("l1", p.ID, domain.ChamberCamara, "1", "PEC 221/2019", "https://www.camara.leg.br/x", "editor-1", t0)
	confirmed.SyncFailed("erro interno com detalhe")
	suggested, _ := domain.NewSuggestedLink("l2", p.ID, domain.ChamberSenado, "2", "PEC 221/2019", "https://www25.senado.leg.br/y", t0)
	principal := mk(t, domain.ChamberCamara, domain.ReviewDraft)
	principal.Kind, principal.Number, principal.Year, principal.Slug = "PEC", 32, 2015, "pec-32-2015"
	attachedReady := mk(t, domain.ChamberCamara, domain.ReviewReady)
	attachedReady.Kind, attachedReady.Number, attachedReady.Slug = "PEC", 8, "pec-8-2025"
	attachedDraft := mk(t, domain.ChamberCamara, domain.ReviewDraft)

	v, ok := domain.PublicView(p, []domain.SourceLink{confirmed, suggested}, &principal, []domain.Proposition{attachedReady, attachedDraft}, domain.Editorial{})
	if !ok || !v.UnderReview {
		t.Fatalf("in_review must be public and under review: %v %+v", ok, v)
	}
	if len(v.Sources) != 1 || v.Sources[0].Chamber != domain.ChamberCamara {
		t.Errorf("only confirmed links are public: %+v", v.Sources)
	}
	if v.AttachedTo == nil || v.AttachedTo.Name != "PEC 32/2015" || v.AttachedTo.Slug != "" {
		t.Errorf("draft principal: name only, no slug: %+v", v.AttachedTo)
	}
	if len(v.Attached) != 1 || v.Attached[0].Slug != "pec-8-2025" {
		t.Errorf("only public attached are listed: %+v", v.Attached)
	}
	// Pronta sem Versão vigente aprovada não deveria existir; se existir,
	// sai como "em análise", só com dados oficiais.
	ready := mk(t, domain.ChamberCamara, domain.ReviewReady)
	if v, ok := domain.PublicView(ready, nil, nil, nil, domain.Editorial{}); !ok || !v.UnderReview || v.ReviewBadge != 0 {
		t.Errorf("ready without current version: %v %+v", ok, v)
	}
}
