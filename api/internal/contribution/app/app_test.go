package app_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
	"github.com/mphennrichs/nelisa/api/internal/contribution/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func as(role auth.Role, id string) context.Context {
	return auth.WithActor(context.Background(), auth.Actor{UserID: id, Role: role})
}

func setup() (*fakes.World, *app.Submit, *app.Lookup) {
	w := fakes.New()
	w.Public["pec-221-2019"] = app.PublicProposition{ID: "p1", Slug: "pec-221-2019", Sphere: domain.SphereFederal}
	submit := app.NewSubmit(fakes.Contributions{W: w}, fakes.Sources{W: w}, fakes.Props{W: w}, fixedClock{now}, rand.Reader)
	lookup := app.NewLookup(fakes.Contributions{W: w}, fakes.Props{W: w})
	return w, submit, lookup
}

func TestSubmitAndLookup(t *testing.T) {
	w, submit, lookup := setup()
	ctx := context.Background()

	code, err := submit.Execute(ctx, app.SubmitInput{
		PropositionSlug: "pec-221-2019",
		Submission: domain.Submission{
			Kind: domain.KindProgressUpdate, Link: "https://g1.globo.com/x", EventType: domain.EventVotePostponed,
			EventDate: "2026-11-20", Note: "adiada",
		},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if len(w.Contributions) != 1 {
		t.Fatalf("contribuições = %d", len(w.Contributions))
	}
	for _, c := range w.Contributions {
		if c.TrackingHash == string(code) || c.TrackingHash != code.Hash() {
			t.Errorf("o banco deve guardar só o hash do código")
		}
		if c.PropositionID != "p1" || c.State != domain.StatePending {
			t.Errorf("contribuição = %+v", c)
		}
	}
	if len(w.Events) != 0 {
		t.Errorf("envio do Leitor não audita: %v", w.Events)
	}

	res, err := lookup.Execute(ctx, " "+code.Display()+" ")
	if err != nil || res.State != domain.StatePending || res.Kind != domain.KindProgressUpdate || res.PropositionSlug != "pec-221-2019" {
		t.Fatalf("lookup = %+v, %v", res, err)
	}
	for _, bad := range []string{"", "AAAA-AAAA-AAAA", "não é código"} {
		if _, err := lookup.Execute(ctx, bad); !errors.Is(err, domain.ErrContributionNotFound) {
			t.Errorf("lookup(%q) = %v", bad, err)
		}
	}
}

func TestSubmitRejects(t *testing.T) {
	w, submit, _ := setup()
	ctx := context.Background()
	cases := map[string]struct {
		in   app.SubmitInput
		want error
	}{
		"proposição não pública": {app.SubmitInput{PropositionSlug: "rascunho", Submission: domain.Submission{
			Kind: domain.KindProgressUpdate, Link: "https://g1.globo.com/x", EventType: domain.EventOther, EventDate: "2026-11-01",
		}}, domain.ErrPropositionNotFound},
		"indicação fora de fonte oficial": {app.SubmitInput{Submission: domain.Submission{
			Kind: domain.KindIndication, Link: "https://exemplo.org/pl", Sphere: domain.SphereMunicipal,
		}}, domain.ErrNotOfficialSource},
		"link http": {app.SubmitInput{Submission: domain.Submission{
			Kind: domain.KindIndication, Link: "http://camara.leg.br", Sphere: domain.SphereFederal,
		}}, domain.ErrInvalidLink},
	}
	for name, tc := range cases {
		if _, err := submit.Execute(ctx, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
	}
	if len(w.Contributions) != 0 {
		t.Errorf("nada deveria ser gravado: %d", len(w.Contributions))
	}

	// Domínio cadastrado passa a valer para a Indicação.
	w.Sources["s1"] = domain.OfficialSource{ID: "s1", Domain: "exemplo.org"}
	if _, err := submit.Execute(ctx, app.SubmitInput{Submission: domain.Submission{
		Kind: domain.KindIndication, Link: "https://www.exemplo.org/pl", Sphere: domain.SphereMunicipal,
	}}); err != nil {
		t.Errorf("indicação de fonte cadastrada: %v", err)
	}
}

func TestSubmitRetriesTakenCode(t *testing.T) {
	w := fakes.New()
	// Fonte de "aleatoriedade" que repete o mesmo código duas vezes e
	// depois muda.
	zeros := make([]byte, 12)
	ones := bytes.Repeat([]byte{1}, 12)
	rnd := bytes.NewReader(append(append(append([]byte{}, zeros...), zeros...), ones...))
	submit := app.NewSubmit(fakes.Contributions{W: w}, fakes.Sources{W: w}, fakes.Props{W: w}, fixedClock{now}, rnd)
	in := app.SubmitInput{Submission: domain.Submission{Kind: domain.KindIndication, Link: "https://camara.leg.br/x", Sphere: domain.SphereFederal}}
	a, err := submit.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := submit.Execute(context.Background(), in)
	if err != nil || a == b {
		t.Fatalf("segundo envio = %q, %v (primeiro %q)", b, err, a)
	}
}

func TestLookupHidesDetails(t *testing.T) {
	w, _, lookup := setup()
	code := domain.TrackingCode("ABCDEFGHJKMN")
	decided := now
	w.Contributions["c1"] = domain.Contribution{
		ID: "c1", Kind: domain.KindIndication, State: domain.StateRejected, TrackingHash: code.Hash(),
		Sphere: domain.SphereFederal, SubmittedAt: now, DecidedBy: "u1", DecidedAt: &decided,
		PublicReason: "duplicate", InternalNote: "spam do mesmo IP", CreatedPropositionID: "p1",
	}
	res, err := lookup.Execute(context.Background(), "abcd-efgh-jkmn")
	if err != nil || res.PublicReason != "duplicate" || res.PropositionSlug != "" {
		t.Fatalf("lookup = %+v, %v", res, err)
	}
}

func TestOfficialSources(t *testing.T) {
	w := fakes.New()
	create := app.NewCreateOfficialSource(fakes.Sources{W: w}, fakes.Tx{W: w}, fakes.Audit{W: w}, fixedClock{now})
	del := app.NewDeleteOfficialSource(fakes.Sources{W: w}, fakes.Tx{W: w}, fakes.Audit{W: w})
	list := app.NewListOfficialSources(fakes.Sources{W: w})
	in := app.CreateOfficialSourceInput{Domain: "CMSP.org", Name: "Câmara Municipal de São Paulo", Sphere: domain.SphereMunicipal}

	if _, err := create.Execute(context.Background(), in); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("sem login: %v", err)
	}
	s, err := create.Execute(as(auth.RoleEditor, "ed"), in)
	if err != nil || s.Domain != "cmsp.org" || s.CreatedBy != "ed" {
		t.Fatalf("create = %+v, %v", s, err)
	}
	if len(w.Events) != 1 || w.Events[0].Action != app.ActionOfficialSourceCreate || !w.Events[0].InTx {
		t.Errorf("auditoria = %+v", w.Events)
	}
	for _, d := range []string{"cmsp.org", "www.cmsp.org", "al.sp.gov.br", "x.leg.br"} {
		dup := in
		dup.Domain = d
		if _, err := create.Execute(as(auth.RoleReviewer, "rv"), dup); !errors.Is(err, domain.ErrOfficialSourceExists) {
			t.Errorf("create %q: %v", d, err)
		}
	}
	ss, err := list.Execute(as(auth.RoleEditor, "ed"))
	if err != nil || len(ss) != 1 {
		t.Errorf("list = %v, %v", ss, err)
	}

	// Remover é só do Admin.
	for _, r := range []auth.Role{auth.RoleEditor, auth.RoleReviewer} {
		if err := del.Execute(as(r, "x"), s.ID); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("delete como %s: %v", r, err)
		}
	}
	if err := del.Execute(as(auth.RoleAdmin, "adm"), s.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(w.Sources) != 0 || len(w.Events) != 2 || w.Events[1].Action != app.ActionOfficialSourceDelete {
		t.Errorf("depois do delete: %v %v", w.Sources, w.Events)
	}
	if err := del.Execute(as(auth.RoleAdmin, "adm"), s.ID); !errors.Is(err, domain.ErrOfficialSourceNotFound) {
		t.Errorf("delete de novo: %v", err)
	}
}
