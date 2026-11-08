package domain_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

func TestNewVersionValidatesLabelAndURL(t *testing.T) {
	v, err := domain.NewVersion("v2", "p1", 2, "  Substitutivo da CCJ ", " https://www.camara.leg.br/sub.pdf ", "ed-1", t0)
	if err != nil || v.Label != "Substitutivo da CCJ" || v.OfficialTextURL != "https://www.camara.leg.br/sub.pdf" ||
		v.State != domain.VersionPending || v.CreatedBy != "ed-1" || v.Seq != 2 {
		t.Fatalf("got %+v, %v", v, err)
	}
	cases := []struct {
		label, url string
		want       error
	}{
		{"", "https://x.leg.br/a", domain.ErrInvalidLabel},
		{strings.Repeat("a", 121), "https://x.leg.br/a", domain.ErrInvalidLabel},
		{"ok", "", domain.ErrInvalidVersionURL},
		{"ok", "javascript:alert(1)", domain.ErrInvalidVersionURL},
		{"ok", "ftp://x.leg.br/a", domain.ErrInvalidVersionURL},
	}
	for _, c := range cases {
		if _, err := domain.NewVersion("v", "p", 1, c.label, c.url, "u", t0); !errors.Is(err, c.want) {
			t.Errorf("%q %q: err = %v, want %v", c.label, c.url, err, c.want)
		}
	}
	// 120 caracteres com acento contam como 120, não como bytes.
	if _, err := domain.NewVersion("v", "p", 1, strings.Repeat("ã", 120), "https://x.leg.br", "u", t0); err != nil {
		t.Errorf("120 runes rejected: %v", err)
	}
}

func TestImportedVersionIsPendingOriginalText(t *testing.T) {
	p := mk(t, domain.ChamberCamara, domain.ReviewDraft)
	p.OfficialTextURL = "https://www.camara.leg.br/inteiro.pdf"
	v := domain.NewImportedVersion("v1", p, t0)
	if v.Seq != 1 || v.Label != "Texto original" || v.State != domain.VersionPending || v.CreatedBy != "" || v.OfficialTextURL != p.OfficialTextURL {
		t.Errorf("got %+v", v)
	}
}

func TestContentEditNormalizeRejectsInsteadOfCutting(t *testing.T) {
	e, err := domain.ContentEdit{Summary: "  linha 1\r\nlinha 2  ", ShortSummary: " curto ", TopicIDs: []string{"a", "b"}}.Normalize()
	if err != nil || e.Summary != "linha 1\nlinha 2" || e.ShortSummary != "curto" || len(e.TopicIDs) != 2 {
		t.Fatalf("got %+v %v", e, err)
	}
	cases := []struct {
		edit domain.ContentEdit
		want error
	}{
		{domain.ContentEdit{Summary: strings.Repeat("é", 5001)}, domain.ErrSummaryTooLong},
		{domain.ContentEdit{ShortSummary: strings.Repeat("x", 281)}, domain.ErrShortSummaryTooLong},
		{domain.ContentEdit{TopicIDs: []string{"a", "b", "c", "d"}}, domain.ErrTooManyTopics},
		{domain.ContentEdit{TopicIDs: []string{"a", "a"}}, domain.ErrDuplicateTopic},
	}
	for _, c := range cases {
		if _, err := c.edit.Normalize(); !errors.Is(err, c.want) {
			t.Errorf("err = %v, want %v", err, c.want)
		}
	}
	if _, err := (domain.ContentEdit{Summary: strings.Repeat("é", 5000), ShortSummary: strings.Repeat("x", 280)}).Normalize(); err != nil {
		t.Errorf("limits are inclusive: %v", err)
	}
}

func TestContentApplyTracksAuthorsAndRevision(t *testing.T) {
	c := domain.NewEmptyContent("v1", t0)
	changed := c.Apply(domain.ContentEdit{Summary: "Resumo", TopicIDs: []string{"t1"}}, "ana", t0.Add(time.Minute))
	if !slices.Equal(changed, []string{"summary", "topic_ids"}) || c.Revision != 1 || !c.HasAuthor("ana") || c.UpdatedBy != "ana" {
		t.Fatalf("first save: %v %+v", changed, c)
	}
	// Sem mudança: ninguém vira autor, a revisão não sobe.
	if changed := c.Apply(domain.ContentEdit{Summary: "Resumo", TopicIDs: []string{"t1"}}, "bia", t0); changed != nil || c.HasAuthor("bia") || c.Revision != 1 {
		t.Fatalf("no-op save: %v %+v", changed, c)
	}
	c.Apply(domain.ContentEdit{Summary: "Resumo", ShortSummary: "Curto", TopicIDs: []string{"t1"}}, "bia", t0)
	c.Apply(domain.ContentEdit{Summary: "Resumo 2", ShortSummary: "Curto", TopicIDs: []string{"t1"}}, "ana", t0)
	if !slices.Equal(c.Authors, []string{"ana", "bia"}) || c.Revision != 3 {
		t.Errorf("authors/revision: %+v", c)
	}
}

func TestCopiedContentNeedsConfirmationAndKeepsAuthors(t *testing.T) {
	prev := domain.NewEmptyContent("v1", t0)
	prev.Apply(domain.ContentEdit{Summary: "S", ShortSummary: "C", TopicIDs: []string{"t1"}}, "ana", t0)
	next := prev.CopyForNewVersion("v2", t0)
	if next.VersionID != "v2" || !next.NeedsConfirmation || next.Revision != 0 || next.Summary != "S" || !next.HasAuthor("ana") {
		t.Fatalf("copy: %+v", next)
	}
	// O próprio salvar não confirma.
	next.Apply(domain.ContentEdit{Summary: "S novo", ShortSummary: "C", TopicIDs: []string{"t1"}}, "bia", t0)
	if !next.NeedsConfirmation {
		t.Error("save must not confirm")
	}
	if err := next.Confirm("caio", t0); err != nil || next.NeedsConfirmation || !next.HasAuthor("caio") || next.Revision != 2 {
		t.Errorf("confirm: %v %+v", err, next)
	}
	if err := next.Confirm("caio", t0); !errors.Is(err, domain.ErrNothingToConfirm) {
		t.Errorf("confirm twice: %v", err)
	}
	// A cópia não compartilha memória com a origem.
	next.Authors[0] = "x"
	if prev.Authors[0] != "ana" {
		t.Error("copy aliases the previous content")
	}
}

func TestMissingForApproval(t *testing.T) {
	topics := map[string]domain.Topic{"t1": {ID: "t1", Name: "Saúde", Active: true}, "t2": {ID: "t2", Name: "Velho", Active: false}}
	codes := func(c domain.Content) []string {
		var out []string
		for _, m := range c.MissingForApproval(topics) {
			if m.Message == "" {
				t.Errorf("missing %q without message", m.Code)
			}
			out = append(out, m.Code)
		}
		return out
	}
	empty := domain.NewEmptyContent("v", t0)
	if got := codes(empty); !slices.Equal(got, []string{"summary", "short_summary", "topics"}) {
		t.Errorf("empty: %v", got)
	}
	full := domain.Content{Summary: "S", ShortSummary: "C", TopicIDs: []string{"t1"}}
	if got := codes(full); len(got) != 0 {
		t.Errorf("full: %v", got)
	}
	withInactive := domain.Content{Summary: "S", ShortSummary: "C", TopicIDs: []string{"t1", "t2"}}
	if got := codes(withInactive); !slices.Equal(got, []string{"inactive_topics"}) {
		t.Errorf("inactive: %v", got)
	}
	unknown := domain.Content{Summary: "S", ShortSummary: "C", TopicIDs: []string{"sumiu"}}
	if got := codes(unknown); !slices.Equal(got, []string{"topics", "inactive_topics"}) {
		t.Errorf("unknown: %v", got)
	}
	copied := full
	copied.NeedsConfirmation = true
	if got := codes(copied); !slices.Equal(got, []string{"needs_confirmation"}) {
		t.Errorf("copied: %v", got)
	}
	blank := domain.Content{Summary: "   ", ShortSummary: "C", TopicIDs: []string{"t1"}}
	if got := codes(blank); !slices.Equal(got, []string{"summary"}) {
		t.Errorf("blank: %v", got)
	}
}

func TestCheckTopicChoice(t *testing.T) {
	topics := map[string]domain.Topic{"on": {ID: "on", Active: true}, "off": {ID: "off", Active: false}}
	if err := domain.CheckTopicChoice([]string{"on"}, nil, topics); err != nil {
		t.Error(err)
	}
	if err := domain.CheckTopicChoice([]string{"off"}, nil, topics); !errors.Is(err, domain.ErrTopicInactive) {
		t.Errorf("new inactive: %v", err)
	}
	// Desativado depois de escolhido continua na Versão.
	if err := domain.CheckTopicChoice([]string{"off", "on"}, []string{"off"}, topics); err != nil {
		t.Errorf("kept inactive: %v", err)
	}
	if err := domain.CheckTopicChoice([]string{"nope"}, nil, topics); !errors.Is(err, domain.ErrTopicNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

func TestTopicName(t *testing.T) {
	tp, err := domain.NewTopic("t", "  Meio   ambiente ", t0)
	if err != nil || tp.Name != "Meio ambiente" || !tp.Active {
		t.Fatalf("%+v %v", tp, err)
	}
	for _, n := range []string{"", "   ", strings.Repeat("a", 61)} {
		if _, err := domain.NewTopic("t", n, t0); !errors.Is(err, domain.ErrInvalidTopicName) {
			t.Errorf("%q: %v", n, err)
		}
	}
}

func TestReviewStateTransitionsForVersions(t *testing.T) {
	p := mk(t, domain.ChamberCamara, domain.ReviewDraft)
	if err := p.MarkReady(t0); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("draft cannot be approved: %v", err)
	}
	p.ReviewState = domain.ReviewInReview
	if err := p.MarkReady(t0); err != nil || p.ReviewState != domain.ReviewReady {
		t.Fatalf("mark ready: %v %s", err, p.ReviewState)
	}
	p.ReturnToReview(t0)
	if p.ReviewState != domain.ReviewInReview {
		t.Errorf("new version must reopen review: %s", p.ReviewState)
	}
	d := mk(t, domain.ChamberCamara, domain.ReviewDraft)
	d.ReturnToReview(t0)
	if d.ReviewState != domain.ReviewDraft {
		t.Errorf("draft stays draft: %s", d.ReviewState)
	}
}

// --- visibilidade do ciclo editorial (#22) ---

const (
	secretPendingSummary = "SEGREDO-RESUMO-EM-ANALISE"
	secretPendingShort   = "SEGREDO-CURTO-EM-ANALISE"
	secretAuthor         = "SEGREDO-AUTOR-ID"
	secretUpdater        = "SEGREDO-QUEM-GRAVOU"
	secretTopic          = "SEGREDO-TEMA-EM-ANALISE"
)

// editorialFixture monta uma Proposição com Versão vigente aprovada e uma
// Versão em análise cheia de segredos.
func editorialFixture(t *testing.T, state domain.ReviewState, withCurrent bool) (domain.Proposition, domain.Editorial) {
	t.Helper()
	p := mk(t, domain.ChamberCamara, state)
	p.OfficialTextURL = "https://www.camara.leg.br/oficial.pdf"
	pending := domain.Version{ID: "v2", PropositionID: p.ID, Seq: 2, Label: "Substitutivo", OfficialTextURL: "https://www.camara.leg.br/sub.pdf", State: domain.VersionPending, CreatedBy: secretAuthor}
	ed := domain.Editorial{
		Versions: []domain.Version{pending},
		Contents: []domain.Content{{
			VersionID: "v2", Summary: secretPendingSummary, ShortSummary: secretPendingShort,
			TopicIDs: []string{"t-secret"}, NeedsConfirmation: true, Authors: []string{secretAuthor}, Revision: 7, UpdatedBy: secretUpdater,
		}},
		Topics: map[string]domain.Topic{
			"t1":       {ID: "t1", Name: "Trabalho", Active: true},
			"t-old":    {ID: "t-old", Name: "Desativado em uso", Active: false},
			"t-secret": {ID: "t-secret", Name: secretTopic, Active: true},
		},
		Badges: map[string]int{"v2": 2}, // nem um Selo indevido faz a Versão em análise sair
	}
	if withCurrent {
		ed.Versions = append(ed.Versions, domain.Version{ID: "v1", PropositionID: p.ID, Seq: 1, Label: "Texto original", OfficialTextURL: "https://www.camara.leg.br/v1.pdf", State: domain.VersionCurrent})
		ed.Contents = append(ed.Contents, domain.Content{
			VersionID: "v1", Summary: "Resumo aprovado", ShortSummary: "Curto aprovado", TopicIDs: []string{"t1", "t-old"},
			Authors: []string{secretAuthor}, Revision: 3, UpdatedBy: secretUpdater,
		})
		ed.Badges["v1"] = 1
	}
	return p, ed
}

func publicJSON(t *testing.T, v domain.PublicProposition) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPublicViewWithoutCurrentShowsOnlyOfficialData(t *testing.T) {
	p, ed := editorialFixture(t, domain.ReviewInReview, false)
	v, ok := domain.PublicView(p, nil, nil, nil, ed)
	if !ok || !v.UnderReview || v.Summary != "" || v.ShortSummary != "" || len(v.Topics) != 0 || v.ReviewBadge != 0 || v.PendingVersion != nil {
		t.Fatalf("in review without current: %+v", v)
	}
	if v.OfficialTextURL != "https://www.camara.leg.br/oficial.pdf" {
		t.Errorf("official link: %q", v.OfficialTextURL)
	}
	for _, leak := range []string{secretPendingSummary, secretPendingShort, secretAuthor, secretUpdater, secretTopic, "Substitutivo", "sub.pdf"} {
		if strings.Contains(publicJSON(t, v), leak) {
			t.Errorf("leaks %q", leak)
		}
	}
	draft, ed := editorialFixture(t, domain.ReviewDraft, true)
	if _, ok := domain.PublicView(draft, nil, nil, nil, ed); ok {
		t.Error("draft with versions must stay 404")
	}
}

func TestPublicViewWithCurrentAndPendingNeverLeaksPending(t *testing.T) {
	p, ed := editorialFixture(t, domain.ReviewInReview, true)
	v, ok := domain.PublicView(p, nil, nil, nil, ed)
	if !ok || v.UnderReview {
		t.Fatalf("with current: %v %+v", ok, v)
	}
	if v.Summary != "Resumo aprovado" || v.ShortSummary != "Curto aprovado" || v.ReviewBadge != 1 {
		t.Errorf("current content: %+v", v)
	}
	if !slices.Equal(v.Topics, []string{"Trabalho", "Desativado em uso"}) {
		t.Errorf("topics (inactive stays in versions that use it): %v", v.Topics)
	}
	if v.OfficialTextURL != "https://www.camara.leg.br/v1.pdf" {
		t.Errorf("reader sees the current version link: %q", v.OfficialTextURL)
	}
	if v.PendingVersion == nil || v.PendingVersion.Label != "Substitutivo" || v.PendingVersion.OfficialTextURL != "https://www.camara.leg.br/sub.pdf" {
		t.Errorf("pending notice: %+v", v.PendingVersion)
	}
	body := publicJSON(t, v)
	// Cada campo interno, um por um: conteúdo da pendente, Autoria, quem
	// gravou, "a confirmar", revisão.
	for _, leak := range []string{secretPendingSummary, secretPendingShort, secretAuthor, secretUpdater, secretTopic,
		"NeedsConfirmation", "needs_confirmation", "Revision", "Authors\":[\"" + secretAuthor} {
		if strings.Contains(body, leak) {
			t.Errorf("leaks %q: %s", leak, body)
		}
	}
}

func TestPublicViewIgnoresContentThatIsNotOfTheCurrentVersion(t *testing.T) {
	p, ed := editorialFixture(t, domain.ReviewReady, true)
	// Uma vigente sem aprovação conhecida (Selo ausente) não mostra nada.
	delete(ed.Badges, "v1")
	if v, _ := domain.PublicView(p, nil, nil, nil, ed); !v.UnderReview || v.Summary != "" {
		t.Errorf("current without badge: %+v", v)
	}
	// Versões de outra Proposição não contam.
	p2, ed2 := editorialFixture(t, domain.ReviewReady, true)
	p2.ID = "outra"
	if v, _ := domain.PublicView(p2, nil, nil, nil, ed2); !v.UnderReview || v.Summary != "" {
		t.Errorf("foreign versions: %+v", v)
	}
	// Selo 2 sai como 2.
	_, ed3 := editorialFixture(t, domain.ReviewReady, true)
	ed3.Badges["v1"] = 2
	if v, _ := domain.PublicView(p, nil, nil, nil, ed3); v.ReviewBadge != 2 {
		t.Errorf("badge 2: %+v", v)
	}
}

func TestFindVersions(t *testing.T) {
	vs := []domain.Version{{ID: "1", State: domain.VersionSuperseded}, {ID: "2", State: domain.VersionCurrent}, {ID: "3", State: domain.VersionPending}}
	c, p := domain.FindVersions(vs)
	if c == nil || c.ID != "2" || p == nil || p.ID != "3" {
		t.Errorf("%+v %+v", c, p)
	}
}
