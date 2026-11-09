package httpapi_test

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
	"github.com/mphennrichs/nelisa/api/internal/contribution/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/contribution/httpapi"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

var t0 = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func fakeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Test-Actor"); v != "" {
			id, role, _ := strings.Cut(v, ":")
			r = r.WithContext(auth.WithActor(r.Context(), auth.Actor{UserID: id, Role: auth.Role(role)}))
		}
		next.ServeHTTP(w, r)
	})
}

func newServer(enabled bool) (*fakes.World, http.Handler) {
	w := fakes.New()
	w.Public["pec-221-2019"] = app.PublicProposition{ID: "p1", Slug: "pec-221-2019", Sphere: domain.SphereFederal}
	clk := &clock.Fixed{T: t0}
	cs, ss, ps := fakes.Contributions{W: w}, fakes.Sources{W: w}, fakes.Props{W: w}
	h := httpapi.New(httpapi.UseCases{
		Submit:               app.NewSubmit(cs, ss, ps, clk, rand.Reader),
		Lookup:               app.NewLookup(cs, ps),
		ListOfficialSources:  app.NewListOfficialSources(ss),
		CreateOfficialSource: app.NewCreateOfficialSource(ss, fakes.Tx{W: w}, fakes.Audit{W: w}, clk),
		DeleteOfficialSource: app.NewDeleteOfficialSource(ss, fakes.Tx{W: w}, fakes.Audit{W: w}),
	}, httpapi.Options{
		Enabled:  enabled,
		LimitKey: func(r *http.Request) (string, error) { return r.Header.Get("X-Test-IP"), nil },
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Route("/api/public", h.RegisterPublic)
	r.Route("/api/team", func(r chi.Router) {
		r.Use(fakeAuth)
		h.RegisterTeam(r)
	})
	return w, r
}

func do(h http.Handler, method, path, body, actor, ip string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if actor != "" {
		r.Header.Set("X-Test-Actor", actor)
	}
	r.Header.Set("X-Test-IP", ip)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

const update = `{"kind":"progress_update","proposition_slug":"pec-221-2019","link":"https://g1.globo.com/x","event_type":"vote_postponed","event_date":"2026-11-20","note":"adiada"}`

func TestSubmitAndLookup(t *testing.T) {
	w, h := newServer(true)

	rec := do(h, http.MethodGet, "/api/public/contributions/availability", "", "", "1.1.1.1")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"open":true`) {
		t.Fatalf("availability = %d %s", rec.Code, rec.Body)
	}

	rec = do(h, http.MethodPost, "/api/public/contributions", update, "", "1.1.1.1")
	if rec.Code != http.StatusCreated || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("submit = %d %s", rec.Code, rec.Body)
	}
	var sub struct {
		TrackingCode string `json:"tracking_code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	if len(sub.TrackingCode) != 14 || len(w.Contributions) != 1 {
		t.Fatalf("código = %q, %d contribuições", sub.TrackingCode, len(w.Contributions))
	}

	rec = do(h, http.MethodPost, "/api/public/contributions/lookup", `{"code":"`+strings.ToLower(sub.TrackingCode)+`"}`, "", "1.1.1.1")
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `"state":"pending"`) || !strings.Contains(body, `"proposition_slug":"pec-221-2019"`) {
		t.Fatalf("lookup = %d %s", rec.Code, body)
	}
	// Nunca devolve o texto enviado.
	if strings.Contains(body, "adiada") || strings.Contains(body, "globo") {
		t.Errorf("lookup vazou o texto do Leitor: %s", body)
	}

	rec = do(h, http.MethodPost, "/api/public/contributions/lookup", `{"code":"AAAA-AAAA-AAAA"}`, "", "1.1.1.1")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "contribution_not_found") {
		t.Errorf("lookup inexistente = %d %s", rec.Code, rec.Body)
	}
}

func TestSubmitValidation(t *testing.T) {
	_, h := newServer(true)
	cases := map[string]string{
		`{"kind":"indication","link":"https://exemplo.org/x","sphere":"municipal"}`:                                                               "not_official_source",
		`{"kind":"indication","link":"http://camara.leg.br","sphere":"federal"}`:                                                                  "invalid_link",
		`{"kind":"source_request","domain":"camara.leg.br","link":"https://camara.leg.br","source_name":"x","sphere":"federal"}`:                  "official_source_exists",
		`{"kind":"progress_update","proposition_slug":"nao-existe","link":"https://g1.globo.com","event_type":"other","event_date":"2026-11-01"}`: "proposition_not_found",
		`{"kind":"nada"}`: "invalid_kind",
	}
	i := 0
	for body, code := range cases {
		i++
		// IPs diferentes para não esbarrar no limite por IP.
		rec := do(h, http.MethodPost, "/api/public/contributions", body, "", "10.0.0."+string(rune('0'+i)))
		if !strings.Contains(rec.Body.String(), code) {
			t.Errorf("%s: %d %s; want %s", body, rec.Code, rec.Body, code)
		}
	}
}

func TestSubmitRateLimit(t *testing.T) {
	_, h := newServer(true)
	for i := range 5 {
		if rec := do(h, http.MethodPost, "/api/public/contributions", update, "", "2.2.2.2"); rec.Code != http.StatusCreated {
			t.Fatalf("envio %d = %d %s", i, rec.Code, rec.Body)
		}
	}
	if rec := do(h, http.MethodPost, "/api/public/contributions", update, "", "2.2.2.2"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("sexto envio = %d", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/api/public/contributions", update, "", "3.3.3.3"); rec.Code != http.StatusCreated {
		t.Errorf("outro IP = %d", rec.Code)
	}
}

func TestDisabled(t *testing.T) {
	w, h := newServer(false)
	rec := do(h, http.MethodGet, "/api/public/contributions/availability", "", "", "1.1.1.1")
	if !strings.Contains(rec.Body.String(), `"open":false`) || !strings.Contains(rec.Body.String(), `"reason":"disabled"`) {
		t.Errorf("availability = %s", rec.Body)
	}
	if rec := do(h, http.MethodPost, "/api/public/contributions", update, "", "1.1.1.1"); rec.Code == http.StatusCreated || len(w.Contributions) != 0 {
		t.Errorf("envio desligado aceitou: %d", rec.Code)
	}
	// A consulta continua no ar.
	code := domain.TrackingCode("ABCDEFGHJKMN")
	w.Contributions["c1"] = domain.Contribution{ID: "c1", Kind: domain.KindIndication, State: domain.StatePending, TrackingHash: code.Hash(), SubmittedAt: t0}
	if rec := do(h, http.MethodPost, "/api/public/contributions/lookup", `{"code":"ABCD-EFGH-JKMN"}`, "", "1.1.1.1"); rec.Code != 200 {
		t.Errorf("lookup desligado = %d %s", rec.Code, rec.Body)
	}
}

func TestOfficialSourcesRoutes(t *testing.T) {
	w, h := newServer(true)
	body := `{"domain":"cmsp.org","name":"Câmara Municipal de SP","sphere":"municipal"}`
	if rec := do(h, http.MethodPost, "/api/team/official-sources", body, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("sem login = %d", rec.Code)
	}
	rec := do(h, http.MethodPost, "/api/team/official-sources", body, "ed:editor", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	var s struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	rec = do(h, http.MethodGet, "/api/team/official-sources", "", "ed:editor", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "cmsp.org") || !strings.Contains(rec.Body.String(), "leg.br") {
		t.Errorf("list = %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, http.MethodDelete, "/api/team/official-sources/"+s.ID, "", "rv:reviewer", ""); rec.Code != http.StatusForbidden {
		t.Errorf("delete como revisor = %d", rec.Code)
	}
	if rec := do(h, http.MethodDelete, "/api/team/official-sources/"+s.ID, "", "adm:admin", ""); rec.Code != http.StatusNoContent || len(w.Sources) != 0 {
		t.Errorf("delete como admin = %d", rec.Code)
	}
}
