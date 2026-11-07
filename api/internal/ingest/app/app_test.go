package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
)

var t0 = time.Date(2019, 11, 26, 12, 0, 0, 0, time.UTC)

type env struct {
	w              *fakes.World
	camara, senado *fakes.Source
	sources        app.Sources
}

// newEnv tem a PEC 221/2019 nas duas Casas (mesma numeração desde 2019).
func newEnv() *env {
	e := &env{w: fakes.NewWorld(), camara: fakes.NewSource(app.ChamberCamara), senado: fakes.NewSource(app.ChamberSenado)}
	e.camara.Put(app.SourceRecord{
		ExternalID: "2192459", Kind: "PEC", Number: 221, Year: 2019, Summary: "Altera a jornada",
		OriginChamber: app.ChamberCamara, Role: "origin", Status: "Remetida ao Senado",
	}, fakes.Progress1("1-100", t0, "Apresentação"), fakes.Progress1("2-200", t0.Add(48*time.Hour), "Aprovada"))
	e.senado.Put(app.SourceRecord{ExternalID: "8441243", Kind: "PEC", Number: 221, Year: 2019, Role: "review"},
		fakes.Progress1("77", t0.Add(72*time.Hour), "Recebida"))
	e.sources = app.NewSources(e.camara, e.senado)
	return e
}

func as(id string, role auth.Role) context.Context {
	return auth.WithActor(context.Background(), auth.Actor{UserID: id, Role: role})
}

var editor = as("editor-1", auth.RoleEditor)

func (e *env) imp() *app.Import {
	return app.NewImport(e.sources, e.w.Catalog(), e.w.Progress(), e.w.Tx(), e.w.Audit())
}
func (e *env) search() *app.SearchSources { return app.NewSearchSources(e.sources, e.w.Catalog()) }
func (e *env) sync() *app.SyncLink {
	return app.NewSyncLink(e.sources, e.w.Catalog(), e.w.Progress(), e.w.Tx())
}

var pec = app.ImportInput{Chamber: "camara", ExternalID: "2192459"}

func TestTeamUseCasesRefuseAnonymousAndNonTeam(t *testing.T) {
	e := newEnv()
	for _, ctx := range []context.Context{context.Background(), as("x", auth.Role("reader"))} {
		_, err := e.imp().Execute(ctx, pec)
		if !errors.Is(err, app.ErrUnauthenticated) && !errors.Is(err, app.ErrForbidden) {
			t.Errorf("import: %v", err)
		}
		_, err = e.search().Execute(ctx, app.SearchSourcesInput{Kind: "PEC", Number: 221, Year: 2019})
		if !errors.Is(err, app.ErrUnauthenticated) && !errors.Is(err, app.ErrForbidden) {
			t.Errorf("search: %v", err)
		}
	}
	if e.camara.Calls["fetch"]+e.camara.Calls["search"] != 0 || e.w.PropositionCount() != 0 {
		t.Error("refused calls reached the source or wrote data")
	}
}

func TestImportCreatesDraftWithConfirmedAndSuggestedLinks(t *testing.T) {
	e := newEnv()
	res, err := e.imp().Execute(editor, pec)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.Suggested != 1 || len(res.SuggestionsUnavailable) != 0 {
		t.Errorf("result = %+v", res)
	}
	links := e.w.Links()
	if len(links) != 2 {
		t.Fatalf("links = %+v", links)
	}
	for _, l := range links {
		if l.Chamber == app.ChamberSenado && l.Confirmed {
			t.Error("the other chamber was auto-confirmed")
		}
		if l.Chamber == app.ChamberCamara && !l.Confirmed {
			t.Error("the imported chamber must be confirmed")
		}
	}
	if e.w.ProgressCount() != 2 {
		t.Errorf("progress = %d", e.w.ProgressCount())
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionImport || ev[0].TargetID != res.PropositionID || !ev[0].InTx || ev[0].ActorID != "editor-1" {
		t.Errorf("events = %+v", ev)
	}
}

func TestImportTwiceReturnsExisting(t *testing.T) {
	e := newEnv()
	first, err := e.imp().Execute(editor, pec)
	if err != nil {
		t.Fatal(err)
	}
	fetches := e.camara.Calls["fetch"]
	second, err := e.imp().Execute(editor, pec)
	if err != nil || second.Created || second.PropositionID != first.PropositionID {
		t.Fatalf("second import: %v %+v", err, second)
	}
	if e.camara.Calls["fetch"] != fetches || e.w.PropositionCount() != 1 || len(e.w.Events()) != 1 {
		t.Error("idempotent import must not fetch, create or audit again")
	}
	// O registro do Senado é sugestão: importar direto manda confirmar lá.
	if _, err := e.imp().Execute(editor, app.ImportInput{Chamber: "senado", ExternalID: "8441243"}); !errors.Is(err, app.ErrAlreadySuggested) {
		t.Errorf("import of a suggested record: %v", err)
	}
}

func TestImportSourceFailureLeavesNoData(t *testing.T) {
	cases := map[string]func(e *env){
		"fetch down":     func(e *env) { e.camara.FetchErr = fakes.Unavailable() },
		"progress down":  func(e *env) { e.camara.ProgressErr = fakes.Unavailable() },
		"progress write": func(e *env) { e.w.ProgressErr = fakes.ErrBoom },
		"audit fails":    func(e *env) { e.w.AuditErr = fakes.ErrBoom },
		"not found":      func(e *env) { e.camara.FetchErr = app.ErrNotFound },
		"record without name": func(e *env) {
			e.camara.Put(app.SourceRecord{ExternalID: "2192459", Kind: "", Number: 0, Year: 0})
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv()
			setup(e)
			if _, err := e.imp().Execute(editor, pec); err == nil {
				t.Fatal("expected error")
			}
			if e.w.PropositionCount() != 0 || len(e.w.Links()) != 0 || e.w.ProgressCount() != 0 || len(e.w.Events()) != 0 {
				t.Errorf("partial data: props=%d links=%d progress=%d events=%d",
					e.w.PropositionCount(), len(e.w.Links()), e.w.ProgressCount(), len(e.w.Events()))
			}
		})
	}
	e := newEnv()
	e.camara.FetchErr = fakes.Unavailable()
	if _, err := e.imp().Execute(editor, pec); !errors.Is(err, app.ErrUnavailable) {
		t.Errorf("unavailable must be reported as such: %v", err)
	}
}

func TestImportWithOtherChamberDownSkipsSuggestions(t *testing.T) {
	e := newEnv()
	e.senado.SearchErr = fakes.Unavailable()
	res, err := e.imp().Execute(editor, pec)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.Suggested != 0 || len(res.SuggestionsUnavailable) != 1 || res.SuggestionsUnavailable[0] != app.ChamberSenado {
		t.Errorf("result = %+v", res)
	}
	if ev := e.w.Events(); len(ev) != 1 || ev[0].Metadata["suggestions_unavailable"] == nil {
		t.Errorf("audit should note the missing chamber: %+v", ev)
	}
}

func TestImportOtherChamberWithoutRecordIsNotAFailure(t *testing.T) {
	e := newEnv()
	e.camara.Put(app.SourceRecord{ExternalID: "1", Kind: "PL", Number: 1, Year: 2026}) // Senado não tem
	res, err := e.imp().Execute(editor, app.ImportInput{Chamber: "camara", ExternalID: "1"})
	if err != nil || res.Suggested != 0 || len(res.SuggestionsUnavailable) != 0 {
		t.Errorf("%v %+v", err, res)
	}
}

func TestImportRejectsBadInput(t *testing.T) {
	e := newEnv()
	if _, err := e.imp().Execute(editor, app.ImportInput{Chamber: "alesp", ExternalID: "1"}); !errors.Is(err, app.ErrUnknownChamber) {
		t.Errorf("unknown chamber: %v", err)
	}
	for _, id := range []string{"", "../x", "a b"} {
		if _, err := e.imp().Execute(editor, app.ImportInput{Chamber: "camara", ExternalID: id}); !errors.Is(err, app.ErrInvalidID) {
			t.Errorf("id %q: %v", id, err)
		}
	}
}

func TestSearchMarksLinkedAndToleratesOneChamberDown(t *testing.T) {
	e := newEnv()
	if _, err := e.imp().Execute(editor, pec); err != nil {
		t.Fatal(err)
	}
	res, err := e.search().Execute(editor, app.SearchSourcesInput{Kind: "pec", Number: 221, Year: 2019})
	if err != nil || len(res) != 2 {
		t.Fatalf("%v %+v", err, res)
	}
	byChamber := map[app.ChamberCode]app.ChamberResult{}
	for _, r := range res {
		byChamber[r.Chamber] = r
	}
	if h := byChamber[app.ChamberCamara].Hits; len(h) != 1 || h[0].LinkedPropositionID == "" || !h[0].LinkConfirmed {
		t.Errorf("camara hit = %+v", h)
	}
	if h := byChamber[app.ChamberSenado].Hits; len(h) != 1 || h[0].LinkedPropositionID == "" || h[0].LinkConfirmed {
		t.Errorf("senado hit must show the suggestion: %+v", h)
	}

	e.senado.SearchErr = fakes.Unavailable()
	res, err = e.search().Execute(editor, app.SearchSourcesInput{Kind: "PL", Number: 9, Year: 2026})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Chamber == app.ChamberSenado && !r.Unavailable {
			t.Error("senado should be marked unavailable")
		}
		if r.Chamber == app.ChamberCamara && (r.Unavailable || len(r.Hits) != 0) {
			t.Errorf("camara not found must be empty, not unavailable: %+v", r)
		}
	}
	if _, err := e.search().Execute(editor, app.SearchSourcesInput{Kind: "P1", Number: 1, Year: 2020}); !errors.Is(err, app.ErrInvalidQuery) {
		t.Errorf("bad query: %v", err)
	}
}

func confirmedLink(t *testing.T, e *env) fakes.Link {
	t.Helper()
	for _, l := range e.w.Links() {
		if l.Confirmed {
			return l
		}
	}
	t.Fatal("no confirmed link")
	return fakes.Link{}
}

func TestSyncIsIdempotent(t *testing.T) {
	e := newEnv()
	if _, err := e.imp().Execute(editor, pec); err != nil {
		t.Fatal(err)
	}
	link := confirmedLink(t, e)
	e.camara.Put(app.SourceRecord{ExternalID: "2192459", Kind: "PEC", Number: 221, Year: 2019, Status: "Promulgada"},
		fakes.Progress1("1-100", t0, "Apresentação"), fakes.Progress1("2-200", t0.Add(48*time.Hour), "Aprovada"),
		fakes.Progress1("3-300", t0.Add(96*time.Hour), "Promulgada"))
	ctx := context.Background()
	first, err := e.sync().Execute(ctx, link.ID)
	if err != nil || first.NewProgress != 1 {
		t.Fatalf("first sync: %v %+v", err, first)
	}
	second, err := e.sync().Execute(ctx, link.ID)
	if err != nil || second.NewProgress != 0 {
		t.Fatalf("second sync: %v %+v", err, second)
	}
	if e.w.ProgressCount() != 3 {
		t.Errorf("progress = %d, want 3 (no duplicates)", e.w.ProgressCount())
	}
	if p := e.w.Prop(link.PropositionID); p.Updated.Status != "Promulgada" || p.Syncs != 2 {
		t.Errorf("proposition = %+v", p)
	}
	if len(e.w.Events()) != 1 {
		t.Error("sync is a system action: no team audit event")
	}
}

func TestSyncSkipsSuggestionsAndMissingLinks(t *testing.T) {
	e := newEnv()
	if _, err := e.imp().Execute(editor, pec); err != nil {
		t.Fatal(err)
	}
	var sug fakes.Link
	for _, l := range e.w.Links() {
		if !l.Confirmed {
			sug = l
		}
	}
	calls := e.senado.Calls["fetch"]
	for _, id := range []string{sug.ID, "sumiu"} {
		res, err := e.sync().Execute(context.Background(), id)
		if err != nil || !res.Skipped {
			t.Errorf("%s: %v %+v", id, err, res)
		}
	}
	if e.senado.Calls["fetch"] != calls {
		t.Error("a suggested link must never reach the source")
	}
}

func TestSyncFailureIsRecordedWithoutPartialData(t *testing.T) {
	e := newEnv()
	if _, err := e.imp().Execute(editor, pec); err != nil {
		t.Fatal(err)
	}
	link := confirmedLink(t, e)
	before := e.w.ProgressCount()
	e.camara.ProgressErr = fakes.Unavailable()
	_, err := e.sync().Execute(context.Background(), link.ID)
	if !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if l := e.w.LinkByID(link.ID); l.LastError == "" || l.Synced != 0 {
		t.Errorf("link = %+v", l)
	}
	if e.w.ProgressCount() != before {
		t.Error("partial progress written")
	}
	e.camara.ProgressErr = nil
	e.camara.FetchErr = app.ErrNotFound
	if _, err := e.sync().Execute(context.Background(), link.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("not found: %v", err)
	}
}

func TestSyncAllSchedulesConfirmedLinksOnly(t *testing.T) {
	e := newEnv()
	if _, err := e.imp().Execute(editor, pec); err != nil {
		t.Fatal(err)
	}
	n, err := app.NewSyncAll(e.w.Catalog(), e.w.Scheduler()).Execute(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("%v %d", err, n)
	}
	if s := e.w.Scheduled(); len(s) != 1 || s[0] != confirmedLink(t, e).ID {
		t.Errorf("scheduled = %v", s)
	}
}
