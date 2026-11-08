package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

var (
	ana      = as("ana", auth.RoleEditor)
	bia      = as("bia", auth.RoleEditor)
	reviewer = as("rev", auth.RoleReviewer)
	admin    = as("adm", auth.RoleAdmin)
)

func (e *env) update() *app.UpdateContent {
	return app.NewUpdateContent(e.w.Repos(), e.w.Claims(), e.w.Tx(), e.w.Audit(), e.clk)
}
func (e *env) confirmContent() *app.ConfirmContent {
	return app.NewConfirmContent(e.w.Repos(), e.w.Claims(), e.w.Tx(), e.w.Audit(), e.clk)
}
func (e *env) createVersion() *app.CreateVersion {
	return app.NewCreateVersion(e.w.Repos(), e.w.Claims(), e.w.Tx(), e.w.Audit(), e.clk)
}
func (e *env) editorial() *app.Editorial { return app.NewEditorial(e.w.Repos(), e.clk) }

// seedTopics cria "Saúde" e "Trabalho" ativos e "Antigo" desativado.
func (e *env) seedTopics() {
	e.w.PutTopic(domain.Topic{ID: "t-saude", Name: "Saúde", Active: true, CreatedAt: t0})
	e.w.PutTopic(domain.Topic{ID: "t-trab", Name: "Trabalho", Active: true, CreatedAt: t0})
	e.w.PutTopic(domain.Topic{ID: "t-old", Name: "Antigo", Active: false, CreatedAt: t0})
}

// pendingOf devolve a Versão em análise.
func (e *env) pendingOf(t *testing.T, pid string) domain.Version {
	t.Helper()
	_, p := domain.FindVersions(e.w.VersionsOf(pid))
	if p == nil {
		t.Fatal("no pending version")
	}
	return *p
}

func (e *env) eventsOf(action string) []fakes.RecordedEvent {
	var out []fakes.RecordedEvent
	for _, ev := range e.w.Events() {
		if ev.Action == action {
			out = append(out, ev)
		}
	}
	return out
}

func fullEdit(pid, vid string, rev int) app.UpdateContentInput {
	return app.UpdateContentInput{
		PropositionID: pid, VersionID: vid, Summary: "Reduz a jornada.", ShortSummary: "Jornada menor.",
		TopicIDs: []string{"t-trab"}, Revision: rev,
	}
}

func TestImportCreatesPendingVersionOneWithEmptyContent(t *testing.T) {
	e := newEnv()
	res := e.importPEC(t)
	vs := e.w.VersionsOf(res.Proposition.ID)
	if len(vs) != 1 || vs[0].Seq != 1 || vs[0].Label != "Texto original" || vs[0].State != domain.VersionPending || vs[0].CreatedBy != "" {
		t.Fatalf("versions = %+v", vs)
	}
	c, ok := e.w.ContentOf(vs[0].ID)
	if !ok || c.Summary != "" || c.Revision != 0 || len(c.Authors) != 0 || c.NeedsConfirmation {
		t.Errorf("content = %+v", c)
	}
}

func TestUpdateContentClaimsAuditsAndAddsAuthor(t *testing.T) {
	e := newEnv()
	e.seedTopics()
	pid := e.importPEC(t).Proposition.ID
	v := e.pendingOf(t, pid)

	c, err := e.update().Execute(ana, fullEdit(pid, v.ID, 0))
	if err != nil {
		t.Fatal(err)
	}
	if c.Revision != 1 || !slices.Equal(c.Authors, []string{"ana"}) || c.Summary != "Reduz a jornada." {
		t.Fatalf("content = %+v", c)
	}
	if cl, ok := e.w.Claim(pid); !ok || cl.UserID != "ana" {
		t.Errorf("free claim must be taken on save: %+v", cl)
	}
	evs := e.eventsOf(app.ActionContentUpdate)
	if len(evs) != 1 || !evs[0].InTx || evs[0].TargetID != v.ID || evs[0].ActorID != "ana" {
		t.Fatalf("audit = %+v", evs)
	}
	fields, _ := evs[0].Metadata["fields"].([]string)
	if !slices.Equal(fields, []string{"summary", "short_summary", "topic_ids"}) {
		t.Errorf("audit fields = %v", evs[0].Metadata)
	}
	for k, val := range evs[0].Metadata {
		if s, ok := val.(string); ok && strings.Contains(s, "Reduz a jornada") {
			t.Errorf("audit must not carry the text (%s)", k)
		}
	}
}

func TestUpdateContentWithSomeoneElsesClaimIsRefused(t *testing.T) {
	e := newEnv()
	e.seedTopics()
	pid := e.importPEC(t).Proposition.ID
	v := e.pendingOf(t, pid)
	e.w.SetClaim(pid, &app.ClaimInfo{UserID: "ana", HolderName: "Ana"})

	_, err := e.update().Execute(bia, fullEdit(pid, v.ID, 0))
	var taken *app.ClaimTakenError
	if !errors.Is(err, app.ErrClaimTaken) || !errors.As(err, &taken) || taken.HolderName != "Ana" {
		t.Fatalf("err = %v", err)
	}
	if c, _ := e.w.ContentOf(v.ID); c.Revision != 0 || c.Summary != "" {
		t.Errorf("content changed: %+v", c)
	}
	if len(e.w.Events()) != 0 {
		t.Errorf("audit on refusal: %+v", e.w.Events())
	}
}

func TestUpdateContentWithStaleRevisionChangesNothing(t *testing.T) {
	e := newEnv()
	e.seedTopics()
	pid := e.importPEC(t).Proposition.ID
	v := e.pendingOf(t, pid)
	if _, err := e.update().Execute(ana, fullEdit(pid, v.ID, 0)); err != nil {
		t.Fatal(err)
	}
	stale := fullEdit(pid, v.ID, 0)
	stale.Summary = "Por cima"
	if _, err := e.update().Execute(ana, stale); !errors.Is(err, domain.ErrStaleContent) {
		t.Fatalf("err = %v", err)
	}
	if c, _ := e.w.ContentOf(v.ID); c.Summary != "Reduz a jornada." || c.Revision != 1 {
		t.Errorf("stale save changed content: %+v", c)
	}
	if n := len(e.eventsOf(app.ActionContentUpdate)); n != 1 {
		t.Errorf("audit events = %d", n)
	}
}

func TestUpdateContentValidation(t *testing.T) {
	e := newEnv()
	e.seedTopics()
	pid := e.importPEC(t).Proposition.ID
	v := e.pendingOf(t, pid)
	cases := []struct {
		name string
		mut  func(*app.UpdateContentInput)
		want error
	}{
		{"summary too long", func(in *app.UpdateContentInput) { in.Summary = strings.Repeat("a", 5001) }, domain.ErrSummaryTooLong},
		{"short too long", func(in *app.UpdateContentInput) { in.ShortSummary = strings.Repeat("a", 281) }, domain.ErrShortSummaryTooLong},
		{"four topics", func(in *app.UpdateContentInput) { in.TopicIDs = []string{"a", "b", "c", "d"} }, domain.ErrTooManyTopics},
		{"unknown topic", func(in *app.UpdateContentInput) { in.TopicIDs = []string{"nao-existe"} }, domain.ErrTopicNotFound},
		{"inactive topic", func(in *app.UpdateContentInput) { in.TopicIDs = []string{"t-old"} }, domain.ErrTopicInactive},
		{"wrong proposition", func(in *app.UpdateContentInput) { in.PropositionID = "outra" }, domain.ErrPropositionNotFound},
		{"unknown version", func(in *app.UpdateContentInput) { in.VersionID = "v-x" }, domain.ErrVersionNotFound},
	}
	for _, c := range cases {
		in := fullEdit(pid, v.ID, 0)
		c.mut(&in)
		if _, err := e.update().Execute(ana, in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := e.update().Execute(context.Background(), fullEdit(pid, v.ID, 0)); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
	if _, ok := e.w.Claim(pid); ok {
		t.Error("failed saves must not keep the claim")
	}
}

func TestCurrentVersionContentIsNotEditable(t *testing.T) {
	e := newEnv()
	e.seedTopics()
	pid := e.importPEC(t).Proposition.ID
	v := e.pendingOf(t, pid)
	v.State = domain.VersionCurrent
	e.w.PutVersion(v)
	if _, err := e.update().Execute(ana, fullEdit(pid, v.ID, 0)); !errors.Is(err, domain.ErrVersionNotEditable) {
		t.Errorf("err = %v", err)
	}
	if _, err := e.confirmContent().Execute(ana, app.ConfirmContentInput{PropositionID: pid, VersionID: v.ID}); !errors.Is(err, domain.ErrVersionNotEditable) {
		t.Errorf("confirm err = %v", err)
	}
}

func TestAuditFailureUndoesContentSaveAndClaim(t *testing.T) {
	e := newEnv()
	e.seedTopics()
	pid := e.importPEC(t).Proposition.ID
	v := e.pendingOf(t, pid)
	e.w.AuditErr = fakes.ErrBoom
	if _, err := e.update().Execute(ana, fullEdit(pid, v.ID, 0)); !errors.Is(err, fakes.ErrBoom) {
		t.Fatalf("err = %v", err)
	}
	if c, _ := e.w.ContentOf(v.ID); c.Revision != 0 {
		t.Errorf("content kept: %+v", c)
	}
	if _, ok := e.w.Claim(pid); ok {
		t.Error("claim kept after rollback")
	}
}

// approve faz o que o módulo review faria: promove a Versão em análise.
func (e *env) approve(t *testing.T, pid, vid string) {
	t.Helper()
	err := e.w.Tx().WithinTx(context.Background(), func(ctx context.Context) error {
		if _, err := e.editorial().LockForApproval(ctx, pid, vid); err != nil {
			return err
		}
		return e.editorial().Promote(ctx, pid, vid)
	})
	if err != nil {
		t.Fatal(err)
	}
	e.w.Badges[vid] = 2
	e.w.SetClaim(pid, nil)
}

func (e *env) readyWithContent(t *testing.T) (string, domain.Version) {
	t.Helper()
	e.seedTopics()
	pid := e.importPEC(t).Proposition.ID
	if _, err := e.submit().Execute(editor, app.SubmitForReviewInput{PropositionID: pid}); err != nil {
		t.Fatal(err)
	}
	v := e.pendingOf(t, pid)
	if _, err := e.update().Execute(ana, fullEdit(pid, v.ID, 0)); err != nil {
		t.Fatal(err)
	}
	e.approve(t, pid, v.ID)
	return pid, v
}

func TestEditorialPromoteMakesCurrentAndReady(t *testing.T) {
	e := newEnv()
	pid, v1 := e.readyWithContent(t)
	p, _ := e.w.Get(pid)
	cur, pend := domain.FindVersions(e.w.VersionsOf(pid))
	if p.ReviewState != domain.ReviewReady || cur == nil || cur.ID != v1.ID || pend != nil {
		t.Fatalf("after approve: %s %+v %+v", p.ReviewState, cur, pend)
	}
	// Uma Versão que não está em análise não é aprovada de novo.
	err := e.w.Tx().WithinTx(context.Background(), func(ctx context.Context) error {
		_, err := e.editorial().LockForApproval(ctx, pid, v1.ID)
		return err
	})
	if !errors.Is(err, domain.ErrVersionNotPending) {
		t.Errorf("approve current again: %v", err)
	}
}

func TestEditorialRefusesDraftPromotion(t *testing.T) {
	e := newEnv()
	pid := e.importPEC(t).Proposition.ID
	v := e.pendingOf(t, pid)
	err := e.w.Tx().WithinTx(context.Background(), func(ctx context.Context) error {
		return e.editorial().Promote(ctx, pid, v.ID)
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("draft promote: %v", err)
	}
	if e.pendingOf(t, pid).State != domain.VersionPending {
		t.Error("draft version promoted")
	}
}

func TestCreateVersionCopiesContentAsNeedsConfirmation(t *testing.T) {
	e := newEnv()
	pid, v1 := e.readyWithContent(t)
	in := app.CreateVersionInput{PropositionID: pid, Label: "Substitutivo da CCJ", OfficialTextURL: "https://www.camara.leg.br/sub.pdf"}
	res, err := e.createVersion().Execute(bia, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Version.Seq != 2 || res.Version.State != domain.VersionPending || res.Version.CreatedBy != "bia" {
		t.Errorf("version = %+v", res.Version)
	}
	if !res.Content.NeedsConfirmation || res.Content.Summary != "Reduz a jornada." || !slices.Equal(res.Content.Authors, []string{"ana"}) {
		t.Errorf("copied content = %+v", res.Content)
	}
	if p, _ := e.w.Get(pid); p.ReviewState != domain.ReviewInReview {
		t.Errorf("new version must reopen review: %s", p.ReviewState)
	}
	if cl, _ := e.w.Claim(pid); cl.UserID != "bia" {
		t.Errorf("creator takes the claim: %+v", cl)
	}
	if c, _ := e.w.ContentOf(v1.ID); c.NeedsConfirmation {
		t.Error("current content must not change")
	}
	if evs := e.eventsOf(app.ActionVersionCreate); len(evs) != 1 || !evs[0].InTx || evs[0].Metadata["seq"] != 2 {
		t.Errorf("audit = %+v", evs)
	}
	// Só uma em análise.
	if _, err := e.createVersion().Execute(bia, in); !errors.Is(err, domain.ErrPendingVersionExists) {
		t.Errorf("second pending: %v", err)
	}
	// Validação de rótulo e link.
	e2 := newEnv()
	pid2, _ := e2.readyWithContent(t)
	for _, bad := range []app.CreateVersionInput{
		{PropositionID: pid2, Label: "", OfficialTextURL: "https://x.leg.br"},
		{PropositionID: pid2, Label: "ok", OfficialTextURL: "javascript:alert(1)"},
	} {
		if _, err := e2.createVersion().Execute(bia, bad); !errors.Is(err, domain.ErrInvalidLabel) && !errors.Is(err, domain.ErrInvalidVersionURL) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestConfirmContentRequiresClaimAndRevision(t *testing.T) {
	e := newEnv()
	pid, _ := e.readyWithContent(t)
	res, err := e.createVersion().Execute(bia, app.CreateVersionInput{PropositionID: pid, Label: "Emenda", OfficialTextURL: "https://x.leg.br/e"})
	if err != nil {
		t.Fatal(err)
	}
	vid := res.Version.ID
	// Ana não tem a Posse (é da Bia).
	if _, err := e.confirmContent().Execute(ana, app.ConfirmContentInput{PropositionID: pid, VersionID: vid}); !errors.Is(err, app.ErrClaimTaken) {
		t.Errorf("confirm with other's claim: %v", err)
	}
	if _, err := e.confirmContent().Execute(bia, app.ConfirmContentInput{PropositionID: pid, VersionID: vid, Revision: 5}); !errors.Is(err, domain.ErrStaleContent) {
		t.Errorf("stale confirm: %v", err)
	}
	c, err := e.confirmContent().Execute(bia, app.ConfirmContentInput{PropositionID: pid, VersionID: vid})
	if err != nil || c.NeedsConfirmation || !slices.Equal(c.Authors, []string{"ana", "bia"}) || c.Revision != 1 {
		t.Fatalf("confirm: %v %+v", err, c)
	}
	if evs := e.eventsOf(app.ActionContentConfirm); len(evs) != 1 || !evs[0].InTx {
		t.Errorf("audit = %+v", evs)
	}
	if _, err := e.confirmContent().Execute(bia, app.ConfirmContentInput{PropositionID: pid, VersionID: vid, Revision: 1}); !errors.Is(err, domain.ErrNothingToConfirm) {
		t.Errorf("confirm twice: %v", err)
	}
}

func TestTopicsAdminOnlyAndAudited(t *testing.T) {
	e := newEnv()
	create := app.NewCreateTopic(e.w.Topics(), e.w.Tx(), e.w.Audit(), e.clk)
	update := app.NewUpdateTopic(e.w.Topics(), e.w.Tx(), e.w.Audit())
	list := app.NewListTopics(e.w.Topics())

	for _, ctx := range []context.Context{ana, reviewer} {
		if _, err := create.Execute(ctx, app.CreateTopicInput{Name: "Saúde"}); !errors.Is(err, app.ErrForbidden) {
			t.Errorf("non-admin create: %v", err)
		}
	}
	tp, err := create.Execute(admin, app.CreateTopicInput{Name: " Saúde "})
	if err != nil || tp.Name != "Saúde" || !tp.Active {
		t.Fatalf("create: %+v %v", tp, err)
	}
	if _, err := create.Execute(admin, app.CreateTopicInput{Name: "SAÚDE"}); !errors.Is(err, domain.ErrTopicNameTaken) {
		t.Errorf("duplicate (case-insensitive): %v", err)
	}
	name, off, on := "Saúde pública", false, true
	if _, err := update.Execute(ana, app.UpdateTopicInput{TopicID: tp.ID, Active: &off}); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("editor deactivate: %v", err)
	}
	tp, err = update.Execute(admin, app.UpdateTopicInput{TopicID: tp.ID, Name: &name, Active: &off})
	if err != nil || tp.Name != "Saúde pública" || tp.Active {
		t.Fatalf("update: %+v %v", tp, err)
	}
	if _, err := update.Execute(admin, app.UpdateTopicInput{TopicID: tp.ID, Active: &on}); err != nil {
		t.Fatal(err)
	}
	if _, err := update.Execute(admin, app.UpdateTopicInput{TopicID: "nao-existe", Active: &on}); !errors.Is(err, domain.ErrTopicNotFound) {
		t.Errorf("unknown topic: %v", err)
	}
	var actions []string
	for _, ev := range e.w.Events() {
		if !ev.InTx {
			t.Errorf("audit outside tx: %+v", ev)
		}
		actions = append(actions, ev.Action)
	}
	if !slices.Equal(actions, []string{"topic.create", "topic.rename", "topic.deactivate", "topic.reactivate"}) {
		t.Errorf("audit = %v", actions)
	}
	ts, err := list.Execute(ana)
	if err != nil || len(ts) != 1 {
		t.Errorf("editor lists topics: %v %v", ts, err)
	}
	if _, err := list.Execute(context.Background()); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("anonymous list: %v", err)
	}
}

func TestGetPropositionDecidesCanEdit(t *testing.T) {
	e := newEnv()
	e.seedTopics()
	pid := e.importPEC(t).Proposition.ID
	v := e.pendingOf(t, pid)
	e.w.Names = map[string]string{"ana": "Ana", "bia": "Bia"}
	get := app.NewGetProposition(e.w.Repos(), e.w.Links(), e.w.Timeline(), e.w.Claims(), e.w.Reviews(), e.w.People())

	d, err := get.Execute(ana, app.GetPropositionInput{PropositionID: pid})
	if err != nil || !d.Editorial.CanEdit || d.Editorial.CanCreateVersion || d.Editorial.Claim != nil {
		t.Fatalf("free claim: %v %+v", err, d.Editorial)
	}
	if _, err := e.update().Execute(ana, fullEdit(pid, v.ID, 0)); err != nil {
		t.Fatal(err)
	}
	d, _ = get.Execute(bia, app.GetPropositionInput{PropositionID: pid})
	if d.Editorial.CanEdit || d.Editorial.Claim == nil || d.Editorial.Claim.HolderName != "Ana" {
		t.Errorf("other's claim: %+v", d.Editorial)
	}
	if d.Editorial.People["ana"] != "Ana" || d.Editorial.Contents[v.ID].Revision != 1 {
		t.Errorf("people/contents: %+v", d.Editorial)
	}
	d, _ = get.Execute(ana, app.GetPropositionInput{PropositionID: pid})
	if !d.Editorial.CanEdit {
		t.Error("holder can edit")
	}
}

func TestPublicUseCasesNeverShowPendingContent(t *testing.T) {
	e := newEnv()
	pid, _ := e.readyWithContent(t)
	res, err := e.createVersion().Execute(bia, app.CreateVersionInput{PropositionID: pid, Label: "Substitutivo", OfficialTextURL: "https://x.leg.br/sub"})
	if err != nil {
		t.Fatal(err)
	}
	edit := fullEdit(pid, res.Version.ID, 0)
	edit.Summary, edit.ShortSummary = "SEGREDO-PENDENTE", "SEGREDO-CURTO"
	if _, err := e.update().Execute(bia, edit); err != nil {
		t.Fatal(err)
	}
	get := app.NewGetPublicProposition(e.w.Repos(), e.w.Links(), e.w.Timeline(), e.w.Reviews())
	d, err := get.Execute(context.Background(), "pec-221-2019")
	if err != nil {
		t.Fatal(err)
	}
	if d.Summary != "Reduz a jornada." || d.ShortSummary != "Jornada menor." || d.ReviewBadge != 2 || d.UnderReview ||
		d.PendingVersion == nil || d.PendingVersion.Label != "Substitutivo" || !slices.Equal(d.Topics, []string{"Trabalho"}) {
		t.Errorf("public detail = %+v", d.PublicProposition)
	}
	list, err := app.NewListPublicPropositions(e.w.Repos(), e.w.Links(), e.w.Reviews()).Execute(context.Background())
	if err != nil || len(list) != 1 || list[0].Summary != "Reduz a jornada." || strings.Contains(list[0].Summary, "SEGREDO") {
		t.Errorf("public list = %+v %v", list, err)
	}
}
