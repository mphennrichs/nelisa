package domain_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain/structuring"
)

func provs(versionID string, text string) []domain.Provision {
	return domain.ProvisionsFrom(versionID, structuring.Parse(text))
}

const law = "Ementa curta.\nO Congresso Nacional decreta:\nCAPÍTULO I\nGERAL\nArt. 1º Um.\nArt. 2º Dois."

func TestPublicViewShowsAnnotatedTextOfCurrentConfirmedVersionOnly(t *testing.T) {
	p, ed := editorialFixture(t, domain.ReviewInReview, true)
	now := time.Now()
	deleted := now
	ed.Structurings = []domain.Structuring{
		{VersionID: "v1", State: domain.StructuringConfirmed, SourceText: "SEGREDO-TEXTO-FONTE"},
		{VersionID: "v2", State: domain.StructuringConfirmed, SourceText: "SEGREDO-FONTE-V2"},
	}
	ed.Provisions = map[string][]domain.Provision{"v1": provs("v1", law), "v2": provs("v2", law+"\nArt. 3º SEGREDO-DISPOSITIVO-V2.")}
	ed.Comments = []domain.Comment{
		{ID: "c1", VersionID: "v1", ProvisionAddress: "art-1", Body: "Comentário público", Authors: []string{secretAuthor}, CreatedBy: secretAuthor},
		{ID: "c2", VersionID: "v1", ProvisionAddress: "art-1", Body: "SEGREDO-A-CONFIRMAR", NeedsConfirmation: true},
		{ID: "c3", VersionID: "v1", ProvisionAddress: "art-2", Body: "SEGREDO-APAGADO", DeletedAt: &deleted},
		{ID: "c4", VersionID: "v2", ProvisionAddress: "art-1", Body: "SEGREDO-RASCUNHO-V2"},
	}
	v, ok := domain.PublicView(p, nil, nil, nil, ed)
	if !ok {
		t.Fatal("deveria ser público")
	}
	var addrs []string
	for _, pr := range v.AnnotatedText {
		addrs = append(addrs, pr.Address)
	}
	if strings.Join(addrs, " ") != "ementa preambulo h-1 art-1 art-2" {
		t.Fatalf("documento comentado: %v", addrs)
	}
	if got := v.AnnotatedText[3].Comments; !slices.Equal(got, []string{"Comentário público"}) {
		t.Fatalf("comentários do art-1: %v", got)
	}
	js := publicJSON(t, v)
	for _, secret := range []string{"SEGREDO", secretAuthor} {
		if strings.Contains(js, secret) {
			t.Errorf("vazou %q: %s", secret, js)
		}
	}

	// Estruturação da vigente não confirmada: sem Documento comentado.
	ed.Structurings[0].State = domain.StructuringReadyToCheck
	if v, _ := domain.PublicView(p, nil, nil, nil, ed); v.AnnotatedText != nil {
		t.Fatalf("não confirmada: %+v", v.AnnotatedText)
	}
	// Sem vigente: nada.
	p2, ed2 := editorialFixture(t, domain.ReviewInReview, false)
	ed2.Structurings, ed2.Provisions, ed2.Comments = ed.Structurings, ed.Provisions, ed.Comments
	if v, _ := domain.PublicView(p2, nil, nil, nil, ed2); v.AnnotatedText != nil {
		t.Fatal("sem vigente não há Documento comentado")
	}
}

func TestCheckCommentable(t *testing.T) {
	ps := provs("v", law)
	ok := domain.Structuring{State: domain.StructuringConfirmed}
	cases := map[string]struct {
		s    domain.Structuring
		addr string
		want error
	}{
		"artigo":         {ok, "art-1", nil},
		"ementa":         {ok, "ementa", nil},
		"capítulo":       {ok, "h-1", domain.ErrProvisionNotCommentable},
		"inexistente":    {ok, "art-9", domain.ErrProvisionNotFound},
		"não confirmada": {domain.Structuring{State: domain.StructuringReadyToCheck}, "art-1", domain.ErrStructuringNotConfirmed},
	}
	for name, c := range cases {
		if err := domain.CheckCommentable(c.s, ps, c.addr); err != c.want {
			t.Errorf("%s: %v, quer %v", name, err, c.want)
		}
	}
}
