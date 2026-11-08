package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

func TestCheckPublishableKeepsPublishedContentComplete(t *testing.T) {
	ok := domain.ContentEdit{Summary: "R", ShortSummary: "C", TopicIDs: []string{"t1"}}
	if err := ok.CheckPublishable(); err != nil {
		t.Fatal(err)
	}
	for name, e := range map[string]domain.ContentEdit{
		"no summary":       {ShortSummary: "C", TopicIDs: []string{"t1"}},
		"no short summary": {Summary: "R", TopicIDs: []string{"t1"}},
		"no topics":        {Summary: "R", ShortSummary: "C"},
		"too many topics":  {Summary: "R", ShortSummary: "C", TopicIDs: []string{"a", "b", "c", "d"}},
	} {
		if err := e.CheckPublishable(); !errors.Is(err, domain.ErrPublishedContentIncomplete) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestPublicViewShowsOnlyErrataOfTheCurrentVersionNewestFirst(t *testing.T) {
	p, ed := editorialFixture(t, domain.ReviewReady, true)
	day := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	ed.Errata = []domain.PublicErratum{
		{VersionID: "v1", ApprovedAt: day, Said: "dizia A", Correct: "correto B"},
		{VersionID: "v1", ApprovedAt: day.Add(48 * time.Hour), Said: "dizia C", Correct: "correto D"},
		// De outra Versão (a em análise, ou uma substituída): nunca sai.
		{VersionID: "v2", ApprovedAt: day, Said: "SEGREDO-OUTRA-VERSAO", Correct: "x"},
	}
	v, ok := domain.PublicView(p, nil, nil, nil, ed)
	if !ok || len(v.Errata) != 2 || v.Errata[0].Said != "dizia C" || v.Errata[1].Said != "dizia A" {
		t.Fatalf("errata: %+v", v.Errata)
	}
	if strings.Contains(publicJSON(t, v), "SEGREDO-OUTRA-VERSAO") {
		t.Error("erratum of another version leaked")
	}
	// Sem vigente (ou sem Selo), nada de Errata: o conteúdo nem sai.
	p2, ed2 := editorialFixture(t, domain.ReviewInReview, false)
	ed2.Errata = []domain.PublicErratum{{VersionID: "v2", ApprovedAt: day, Said: "x", Correct: "y"}}
	if v, _ := domain.PublicView(p2, nil, nil, nil, ed2); len(v.Errata) != 0 || v.Errata == nil {
		t.Errorf("errata without current: %+v", v.Errata)
	}
}
