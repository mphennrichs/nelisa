package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

type env struct {
	w   *fakes.World
	clk *clock.Fixed
}

func newEnv() *env { return &env{w: fakes.NewWorld(), clk: &clock.Fixed{T: t0}} }

func as(id string, role auth.Role) context.Context {
	return auth.WithActor(context.Background(), auth.Actor{UserID: id, Role: role})
}

var editor = as("editor-1", auth.RoleEditor)

func (e *env) catalog() *app.Catalog {
	return app.NewCatalog(e.w.Repos(), e.w.Links(), e.w.StructuringScheduler(), e.clk)
}
func (e *env) confirm() *app.ConfirmLink {
	return app.NewConfirmLink(e.w.Links(), e.w.Tx(), e.w.Audit(), e.w.Scheduler(), e.clk)
}
func (e *env) reject() *app.RejectLink { return app.NewRejectLink(e.w.Links(), e.w.Tx(), e.w.Audit()) }
func (e *env) submit() *app.SubmitForReview {
	return app.NewSubmitForReview(e.w.Props(), e.w.Tx(), e.w.Audit(), e.clk)
}

// importPEC cria a PEC 221/2019 pela Câmara, com o Senado como candidato,
// do jeito que o ingest chama o Catalog.
func (e *env) importPEC(t *testing.T) app.CreateImportedResult {
	t.Helper()
	var res app.CreateImportedResult
	err := e.w.Tx().WithinTx(context.Background(), func(ctx context.Context) error {
		var err error
		res, err = e.catalog().CreateImported(ctx, app.CreateImportedInput{
			Chamber: domain.ChamberCamara, ExternalID: "2192459", SourceName: "PEC 221/2019",
			SourceURL: "https://www.camara.leg.br/proposicoesWeb/fichadetramitacao?idProposicao=2192459",
			Kind:      "PEC", Number: 221, Year: 2019, OriginChamber: domain.ChamberCamara,
			Data:     domain.OfficialData{OfficialSummary: "Altera a jornada de trabalho", Status: "Em tramitação"},
			EditorID: "editor-1",
			Candidates: []app.Candidate{
				{Chamber: domain.ChamberSenado, ExternalID: "8441243", SourceName: "PEC 221/2019", SourceURL: "https://www25.senado.leg.br/x"},
				{Chamber: domain.ChamberCamara, ExternalID: "999"}, // mesma Casa: ignorado
			},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func suggestedLink(t *testing.T, res app.CreateImportedResult) domain.SourceLink {
	t.Helper()
	if len(res.Suggested) != 1 {
		t.Fatalf("suggested = %+v", res.Suggested)
	}
	return res.Suggested[0]
}

// --- autorização ---

func TestTeamUseCasesRefuseAnonymousAndNonTeam(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	sug := suggestedLink(t, res)
	pid := res.Proposition.ID
	calls := map[string]func(ctx context.Context) error{
		"confirm": func(ctx context.Context) error {
			_, err := e.confirm().Execute(ctx, app.ConfirmLinkInput{PropositionID: pid, LinkID: sug.ID})
			return err
		},
		"reject": func(ctx context.Context) error {
			return e.reject().Execute(ctx, app.RejectLinkInput{PropositionID: pid, LinkID: sug.ID})
		},
		"submit": func(ctx context.Context) error {
			_, err := e.submit().Execute(ctx, app.SubmitForReviewInput{PropositionID: pid})
			return err
		},
		"list": func(ctx context.Context) error {
			_, err := app.NewListPropositions(e.w.Props(), e.w.Links()).Execute(ctx, app.ListPropositionsInput{})
			return err
		},
		"get": func(ctx context.Context) error {
			_, err := app.NewGetProposition(e.w.Repos(), e.w.Links(), e.w.Timeline(), e.w.Claims(), e.w.Reviews(), e.w.People()).Execute(ctx, app.GetPropositionInput{PropositionID: pid})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(context.Background()); !errors.Is(err, app.ErrUnauthenticated) {
				t.Errorf("anonymous reader: err = %v", err)
			}
			if err := call(as("x", auth.Role("reader"))); !errors.Is(err, app.ErrForbidden) {
				t.Errorf("unknown role: err = %v", err)
			}
		})
	}
	if len(e.w.Events()) != 0 || len(e.w.Scheduled()) != 0 {
		t.Errorf("refused calls must not change anything: %+v", e.w.Events())
	}
	if l := e.w.AllLinks(); l[0].State == l[1].State {
		t.Errorf("links changed: %+v", l)
	}
}

// --- Importação (Catalog) ---

func TestCreateImportedNeverConfirmsSuggestions(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	p := res.Proposition
	if p.ReviewState != domain.ReviewDraft || p.Slug != "pec-221-2019" || p.Name() != "PEC 221/2019" {
		t.Errorf("proposition = %+v", p)
	}
	if res.Link.State != domain.LinkConfirmed || res.Link.ConfirmedBy != "editor-1" || res.Link.LastSyncedAt == nil {
		t.Errorf("imported link = %+v", res.Link)
	}
	var confirmed, suggested int
	for _, l := range e.w.AllLinks() {
		switch l.State {
		case domain.LinkConfirmed:
			confirmed++
			if l.Chamber != domain.ChamberCamara {
				t.Errorf("only the imported chamber is confirmed, got %+v", l)
			}
		case domain.LinkSuggested:
			suggested++
			if l.ConfirmedAt != nil || l.ConfirmedBy != "" {
				t.Errorf("suggestion carries confirmation: %+v", l)
			}
		}
	}
	if confirmed != 1 || suggested != 1 {
		t.Errorf("confirmed=%d suggested=%d", confirmed, suggested)
	}
}

func TestCreateImportedSlugCollisionAndCandidateAlreadyConfirmed(t *testing.T) {
	e := newEnv()
	e.importPEC(t)
	// Outra "PEC 221/2019", iniciada no Senado (numeração antiga separada),
	// cujo candidato na Câmara já está confirmado na primeira.
	var res app.CreateImportedResult
	err := e.w.Tx().WithinTx(context.Background(), func(ctx context.Context) error {
		var err error
		res, err = e.catalog().CreateImported(ctx, app.CreateImportedInput{
			Chamber: domain.ChamberSenado, ExternalID: "555", Kind: "PEC", Number: 221, Year: 2019,
			OriginChamber: domain.ChamberSenado, EditorID: "editor-1",
			Candidates: []app.Candidate{{Chamber: domain.ChamberCamara, ExternalID: "2192459"}},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Proposition.Slug != "pec-221-2019-senado" {
		t.Errorf("slug = %q", res.Proposition.Slug)
	}
	if len(res.Suggested) != 0 {
		t.Errorf("a record confirmed elsewhere must not be suggested: %+v", res.Suggested)
	}
}

func TestCreateImportedResolvesAttachmentBothWays(t *testing.T) {
	e := newEnv()
	ctx := context.Background()
	// A apensada chega primeiro: a principal ainda não existe.
	var attached app.CreateImportedResult
	_ = e.w.Tx().WithinTx(ctx, func(ctx context.Context) error {
		var err error
		attached, err = e.catalog().CreateImported(ctx, app.CreateImportedInput{
			Chamber: domain.ChamberCamara, ExternalID: "2487436", Kind: "PEC", Number: 8, Year: 2025,
			OriginChamber: domain.ChamberCamara, EditorID: "editor-1",
			Data: domain.OfficialData{AttachedToName: "PEC 221/2019", Status: "Prejudicada"},
		})
		return err
	})
	if attached.Proposition.AttachedToID != nil || !attached.Proposition.Superseded {
		t.Fatalf("attached before principal: %+v", attached.Proposition)
	}
	principal := e.importPEC(t)
	got, _ := e.w.Get(attached.Proposition.ID)
	if got.AttachedToID == nil || *got.AttachedToID != principal.Proposition.ID {
		t.Errorf("attachment not resolved when principal arrived: %+v", got)
	}
}

// --- Vínculos ---

func TestConfirmLinkConfirmsAuditsAndSchedulesSync(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	sug := suggestedLink(t, res)
	e.clk.Advance(time.Hour)
	l, err := e.confirm().Execute(editor, app.ConfirmLinkInput{PropositionID: res.Proposition.ID, LinkID: sug.ID})
	if err != nil {
		t.Fatal(err)
	}
	if l.State != domain.LinkConfirmed || l.ConfirmedBy != "editor-1" || !l.ConfirmedAt.Equal(e.clk.Now()) {
		t.Errorf("link = %+v", l)
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionLinkConfirm || ev[0].TargetID != sug.ID || !ev[0].InTx {
		t.Errorf("events = %+v", ev)
	}
	if s := e.w.Scheduled(); len(s) != 1 || s[0] != sug.ID {
		t.Errorf("scheduled = %v", s)
	}
	// Confirmar de novo: não é mais sugestão.
	if _, err := e.confirm().Execute(editor, app.ConfirmLinkInput{PropositionID: res.Proposition.ID, LinkID: sug.ID}); !errors.Is(err, domain.ErrLinkNotSuggested) {
		t.Errorf("confirm twice: %v", err)
	}
}

func TestConfirmLinkRefusesRecordConfirmedElsewhere(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	sug := suggestedLink(t, res)
	// Outra Proposição já tem o registro do Senado confirmado.
	other, _ := domain.NewConfirmedLink("l-other", "p-other", domain.ChamberSenado, sug.ExternalID, "", "", "editor-2", t0)
	e.w.PutLink(other)
	_, err := e.confirm().Execute(editor, app.ConfirmLinkInput{PropositionID: res.Proposition.ID, LinkID: sug.ID})
	if !errors.Is(err, domain.ErrLinkTaken) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := e.w.Links().FindByID(context.Background(), sug.ID); got.State != domain.LinkSuggested {
		t.Errorf("link changed: %+v", got)
	}
	if len(e.w.Events()) != 0 || len(e.w.Scheduled()) != 0 {
		t.Error("refused confirmation left audit or job behind")
	}
}

func TestConfirmLinkRollsBackWhenAuditOrQueueFails(t *testing.T) {
	for name, set := range map[string]func(*fakes.World){
		"audit": func(w *fakes.World) { w.AuditErr = fakes.ErrBoom },
		"queue": func(w *fakes.World) { w.ScheduleErr = fakes.ErrBoom },
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv()
			res := e.importPEC(t)
			sug := suggestedLink(t, res)
			set(e.w)
			if _, err := e.confirm().Execute(editor, app.ConfirmLinkInput{PropositionID: res.Proposition.ID, LinkID: sug.ID}); !errors.Is(err, fakes.ErrBoom) {
				t.Fatalf("err = %v", err)
			}
			if got, _ := e.w.Links().FindByID(context.Background(), sug.ID); got.State != domain.LinkSuggested {
				t.Errorf("confirmation must roll back: %+v", got)
			}
			if len(e.w.Events()) != 0 || len(e.w.Scheduled()) != 0 {
				t.Errorf("events=%v scheduled=%v", e.w.Events(), e.w.Scheduled())
			}
		})
	}
}

func TestConfirmLinkOfAnotherPropositionIsNotFound(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	sug := suggestedLink(t, res)
	if _, err := e.confirm().Execute(editor, app.ConfirmLinkInput{PropositionID: "outra", LinkID: sug.ID}); !errors.Is(err, domain.ErrLinkNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestRejectLink(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	sug := suggestedLink(t, res)
	if err := e.reject().Execute(editor, app.RejectLinkInput{PropositionID: res.Proposition.ID, LinkID: res.Link.ID}); !errors.Is(err, domain.ErrLinkNotSuggested) {
		t.Errorf("rejecting a confirmed link: %v", err)
	}
	if err := e.reject().Execute(editor, app.RejectLinkInput{PropositionID: res.Proposition.ID, LinkID: sug.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.w.Links().FindByID(context.Background(), sug.ID); !errors.Is(err, domain.ErrLinkNotFound) {
		t.Errorf("suggestion should be gone: %v", err)
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionLinkReject || ev[0].Metadata["external_id"] != sug.ExternalID || !ev[0].InTx {
		t.Errorf("events = %+v", ev)
	}
}

// --- Revisão ---

func TestSubmitForReview(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	pid := res.Proposition.ID
	p, err := e.submit().Execute(editor, app.SubmitForReviewInput{PropositionID: pid})
	if err != nil || p.ReviewState != domain.ReviewInReview {
		t.Fatalf("submit: %v %+v", err, p)
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionSubmitForReview || ev[0].Metadata["from"] != "draft" || ev[0].Metadata["to"] != "in_review" {
		t.Errorf("events = %+v", ev)
	}
	if _, err := e.submit().Execute(editor, app.SubmitForReviewInput{PropositionID: pid}); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("submit from in_review: %v", err)
	}
	if len(e.w.Events()) != 1 {
		t.Error("refused transition must not audit")
	}
	if _, err := e.submit().Execute(editor, app.SubmitForReviewInput{PropositionID: "nao-existe"}); !errors.Is(err, domain.ErrPropositionNotFound) {
		t.Errorf("unknown proposition: %v", err)
	}
}

// --- Sincronização (Catalog) ---

func TestApplySyncKeepsReviewStateAndRefusesSuggestions(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	sug := suggestedLink(t, res)
	if _, err := e.submit().Execute(editor, app.SubmitForReviewInput{PropositionID: res.Proposition.ID}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := e.catalog().ApplySync(ctx, app.ApplySyncInput{LinkID: sug.ID}); !errors.Is(err, domain.ErrLinkNotFound) {
		t.Errorf("a suggested link must never sync: %v", err)
	}
	later := t0.Add(24 * time.Hour)
	e.clk.Advance(time.Hour)
	for range 2 {
		if err := e.catalog().ApplySync(ctx, app.ApplySyncInput{
			LinkID: res.Link.ID, LatestEventAt: &later, ReportsAttachment: true,
			Data: domain.OfficialData{OfficialSummary: "Altera a jornada de trabalho", Status: "Promulgada"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := e.w.Get(res.Proposition.ID)
	if p.ReviewState != domain.ReviewInReview || p.Status != "Promulgada" {
		t.Errorf("after sync: %+v", p)
	}
	l, _ := e.w.Links().FindByID(ctx, res.Link.ID)
	if l.LastSyncedAt == nil || !l.LastSyncedAt.Equal(e.clk.Now()) {
		t.Errorf("link = %+v", l)
	}
	if err := e.catalog().RecordSyncFailure(ctx, res.Link.ID, "fonte fora do ar"); err != nil {
		t.Fatal(err)
	}
	l, _ = e.w.Links().FindByID(ctx, res.Link.ID)
	if l.LastSyncError != "fonte fora do ar" || l.LastSyncedAt == nil {
		t.Errorf("failure must keep last success: %+v", l)
	}
}

// --- camada pública ---

func TestPublicLayerHidesDraftsAndInternalData(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	list := app.NewListPublicPropositions(e.w.Repos(), e.w.Links(), e.w.Reviews())
	get := app.NewGetPublicProposition(e.w.Repos(), e.w.Links(), e.w.Timeline(), e.w.Reviews())
	ctx := context.Background()

	if ps, err := list.Execute(ctx); err != nil || len(ps) != 0 {
		t.Fatalf("draft listed: %v %+v", err, ps)
	}
	if _, err := get.Execute(ctx, "pec-221-2019"); !errors.Is(err, domain.ErrPropositionNotFound) {
		t.Fatalf("draft readable: %v", err)
	}

	e.w.SetTimeline(res.Proposition.ID, []app.TimelineEntry{{Chamber: "camara", At: t0, Description: "Apresentação"}})
	if _, err := e.submit().Execute(editor, app.SubmitForReviewInput{PropositionID: res.Proposition.ID}); err != nil {
		t.Fatal(err)
	}
	d, err := get.Execute(ctx, "pec-221-2019")
	if err != nil {
		t.Fatal(err)
	}
	if !d.UnderReview || d.Name != "PEC 221/2019" || len(d.Timeline) != 1 {
		t.Errorf("public detail = %+v", d)
	}
	if len(d.Sources) != 1 || d.Sources[0].Chamber != domain.ChamberCamara {
		t.Errorf("suggested link leaked into public sources: %+v", d.Sources)
	}
	if ps, _ := list.Execute(ctx); len(ps) != 1 || !ps[0].UnderReview {
		t.Errorf("list = %+v", ps)
	}
}
