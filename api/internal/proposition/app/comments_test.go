package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// structured devolve a Versão em análise de uma Proposição enviada para
// revisão, com o texto colado (pecText) e a Estruturação confirmada por who.
func (e *env) structured(t *testing.T, who context.Context) domain.Version {
	t.Helper()
	v := e.importWithURL(t, "")
	if _, err := e.submit().Execute(editor, app.SubmitForReviewInput{PropositionID: v.PropositionID}); err != nil {
		t.Fatal(err)
	}
	e.reprocessAndConfirm(t, who, v, pecText)
	return v
}

func (e *env) reprocessAndConfirm(t *testing.T, who context.Context, v domain.Version, text string) {
	t.Helper()
	s, _ := e.w.StructuringOf(v.ID)
	view, err := app.NewReprocessStructuring(e.structEdit()).Execute(who, app.ReprocessStructuringInput{
		PropositionID: v.PropositionID, VersionID: v.ID, SourceText: text, Revision: s.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewConfirmStructuring(e.structEdit()).Execute(who, app.ConfirmStructuringInput{
		PropositionID: v.PropositionID, VersionID: v.ID, Revision: view.Revision,
	}); err != nil {
		t.Fatal(err)
	}
}

func (e *env) comment(t *testing.T, who context.Context, v domain.Version, addr, body string) domain.Comment {
	t.Helper()
	c, err := app.NewCreateComment(e.structEdit()).Execute(who, app.CreateCommentInput{
		PropositionID: v.PropositionID, VersionID: v.ID, ProvisionAddress: addr, Body: body,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCommentNeedsConfirmedStructuring(t *testing.T) {
	e := newEnv()
	v := e.importWithURL(t, "")
	_, err := app.NewCreateComment(e.structEdit()).Execute(ana, app.CreateCommentInput{
		PropositionID: v.PropositionID, VersionID: v.ID, ProvisionAddress: "art-1", Body: "Muda a jornada.",
	})
	if !errors.Is(err, domain.ErrStructuringNotConfirmed) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateComment(t *testing.T) {
	e := newEnv()
	v := e.structured(t, ana)
	for _, tt := range []struct {
		addr, body string
		want       error
	}{
		{"art-9", "x", domain.ErrProvisionNotFound},
		{"art-1", "   ", domain.ErrEmptyComment},
		{"art-1", strings.Repeat("a", domain.MaxCommentLen+1), domain.ErrCommentTooLong},
	} {
		_, err := app.NewCreateComment(e.structEdit()).Execute(ana, app.CreateCommentInput{
			PropositionID: v.PropositionID, VersionID: v.ID, ProvisionAddress: tt.addr, Body: tt.body,
		})
		if !errors.Is(err, tt.want) {
			t.Errorf("%s %q: %v, quer %v", tt.addr, tt.body[:min(len(tt.body), 10)], err, tt.want)
		}
	}

	c := e.comment(t, ana, v, "art-1", "  Muda a jornada.\r\n\r\nE o repouso.  ")
	if c.Body != "Muda a jornada.\n\nE o repouso." || !slices.Equal(c.Authors, []string{"ana"}) || c.NeedsConfirmation {
		t.Fatalf("comentário = %+v", c)
	}
	if evs := e.eventsOf(app.ActionCommentCreate); len(evs) != 1 || !evs[0].InTx || evs[0].Metadata["provision_address"] != "art-1" {
		t.Fatalf("auditoria: %+v", evs)
	}
	// Outra pessoa com a Posse da Ana: 409.
	_, err := app.NewCreateComment(e.structEdit()).Execute(bia, app.CreateCommentInput{
		PropositionID: v.PropositionID, VersionID: v.ID, ProvisionAddress: "art-2", Body: "Outro.",
	})
	var taken *app.ClaimTakenError
	if !errors.As(err, &taken) {
		t.Fatalf("err = %v", err)
	}
}

func TestCommentAuthorCannotApprove(t *testing.T) {
	e := newEnv()
	v := e.structured(t, ana)
	e.w.SetClaim(v.PropositionID, nil)
	e.comment(t, bia, v, "art-2", "Vigência imediata.")
	s, err := e.editorial().PendingSubject(context.Background(), v.PropositionID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(s.Authors, "bia") {
		t.Fatalf("quem comenta entra na Autoria: %v", s.Authors)
	}
}

func TestUpdateAndDeleteComment(t *testing.T) {
	e := newEnv()
	v := e.structured(t, ana)
	c := e.comment(t, ana, v, "art-1", "Primeiro.")
	upd := app.NewUpdateComment(e.structEdit())
	if _, err := upd.Execute(ana, app.UpdateCommentInput{
		PropositionID: v.PropositionID, VersionID: v.ID, CommentID: c.ID, Body: "Segundo.", Revision: c.Revision + 1,
	}); !errors.Is(err, domain.ErrStaleContent) {
		t.Fatalf("revisão velha: %v", err)
	}
	c2, err := upd.Execute(ana, app.UpdateCommentInput{
		PropositionID: v.PropositionID, VersionID: v.ID, CommentID: c.ID, Body: "Segundo.", Revision: c.Revision,
	})
	if err != nil || c2.Body != "Segundo." || c2.Revision != 1 {
		t.Fatalf("c2 %+v err %v", c2, err)
	}
	del := app.NewDeleteComment(e.structEdit())
	if _, err := del.Execute(ana, app.CommentActionInput{
		PropositionID: v.PropositionID, VersionID: v.ID, CommentID: c.ID, Revision: c2.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	if cs := e.w.CommentsOf(v.ID); len(cs) != 1 || cs[0].DeletedAt == nil {
		t.Fatalf("apagar é soft: %+v", cs)
	}
	// Apagado não se edita.
	if _, err := upd.Execute(ana, app.UpdateCommentInput{
		PropositionID: v.PropositionID, VersionID: v.ID, CommentID: c.ID, Body: "x", Revision: 2,
	}); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Fatalf("apagado: %v", err)
	}
}

func TestReprocessCannotDropCommentedProvision(t *testing.T) {
	e := newEnv()
	v := e.structured(t, ana)
	c := e.comment(t, ana, v, "art-2", "Vigência.")
	without := strings.Replace(pecText, "Art. 2º Esta Emenda entra em vigor na data de sua publicação.\n", "", 1)
	s, _ := e.w.StructuringOf(v.ID)
	_, err := app.NewReprocessStructuring(e.structEdit()).Execute(ana, app.ReprocessStructuringInput{
		PropositionID: v.PropositionID, VersionID: v.ID, SourceText: without, Revision: s.Revision,
	})
	var gone *domain.AnchorsRemovedError
	if !errors.As(err, &gone) || !slices.Equal(gone.Addresses, []string{"art-2"}) {
		t.Fatalf("err = %v", err)
	}
	if len(e.w.ProvisionsOf(v.ID)) != 5 {
		t.Fatal("nada pode mudar")
	}
	// Apagado o Comentário, pode.
	if _, err := app.NewDeleteComment(e.structEdit()).Execute(ana, app.CommentActionInput{
		PropositionID: v.PropositionID, VersionID: v.ID, CommentID: c.ID, Revision: c.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	s, _ = e.w.StructuringOf(v.ID)
	if _, err := app.NewReprocessStructuring(e.structEdit()).Execute(ana, app.ReprocessStructuringInput{
		PropositionID: v.PropositionID, VersionID: v.ID, SourceText: without, Revision: s.Revision,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNewVersionCarriesCommentsOfUnchangedProvisions(t *testing.T) {
	e := newEnv()
	v1 := e.structured(t, ana)
	e.comment(t, ana, v1, "art-1", "Sobre o art. 1º.")
	e.comment(t, ana, v1, "art-2", "Sobre a vigência.")
	e.approve(t, v1.PropositionID, v1.ID)

	res, err := e.createVersion().Execute(bia, app.CreateVersionInput{
		PropositionID: v1.PropositionID, Label: "Substitutivo", OfficialTextURL: "https://sapl.exemplo.gov.br/sub.pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	v2 := res.Version
	// No substitutivo, o art. 2º mudou; o art. 1º não.
	changed := strings.Replace(pecText, "na data de sua publicação", "noventa dias após a publicação", 1)
	e.reprocessAndConfirm(t, bia, v2, changed)

	cs := e.w.CommentsOf(v2.ID)
	if len(cs) != 1 || cs[0].ProvisionAddress != "art-1" || !cs[0].NeedsConfirmation || cs[0].CopiedFrom == "" {
		t.Fatalf("trazidos = %+v", cs)
	}
	missing := func() []string {
		s, _ := e.editorial().PendingSubject(context.Background(), v2.PropositionID)
		var out []string
		for _, m := range s.Missing {
			out = append(out, m.Code)
		}
		return out
	}
	if !slices.Contains(missing(), domain.MissingCommentsUnconfirmed) {
		t.Fatal("Comentário trazido deveria bloquear a aprovação")
	}

	// Reprocessar e confirmar de novo não duplica.
	e.reprocessAndConfirm(t, bia, v2, changed+"\n")
	if n := len(e.w.CommentsOf(v2.ID)); n != 1 {
		t.Fatalf("duplicou: %d", n)
	}

	if _, err := app.NewConfirmComment(e.structEdit()).Execute(bia, app.CommentActionInput{
		PropositionID: v2.PropositionID, VersionID: v2.ID, CommentID: cs[0].ID, Revision: cs[0].Revision,
	}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(missing(), domain.MissingCommentsUnconfirmed) {
		t.Fatal("confirmado, não bloqueia mais")
	}
}
