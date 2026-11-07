package httpsrc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
)

func newClient(t *testing.T, h http.HandlerFunc, cfg Config) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(srv.Client(), srv.URL, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGetJSONOK(t *testing.T) {
	var ua, accept string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		ua, accept = r.UserAgent(), r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{"a":1}`))
	}, Config{})
	var out struct{ A int }
	if err := c.GetJSON(context.Background(), c.URL("/x", nil), &out); err != nil {
		t.Fatal(err)
	}
	if out.A != 1 {
		t.Fatalf("A = %d", out.A)
	}
	if ua != UserAgent || accept != "application/json" {
		t.Fatalf("cabeçalhos: UA %q, Accept %q", ua, accept)
	}
}

func TestGetJSONErrors(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
		want error
	}{
		{"404", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }, app.ErrNotFound},
		{"500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }, app.ErrUnavailable},
		{"503 sem Retry-After", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }, app.ErrUnavailable},
		{"JSON lixo", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`<html>oi</html>`)) }, app.ErrUnavailable},
		{"JSON cortado", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"a":`)) }, app.ErrUnavailable},
		{"corpo grande demais", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"a":"` + strings.Repeat("x", 2048) + `"}`))
		}, app.ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, tc.h, Config{MaxBody: 1024})
			var out map[string]any
			err := c.GetJSON(context.Background(), c.URL("/x", nil), &out)
			if !errors.Is(err, tc.want) {
				t.Fatalf("erro = %v, quero %v", err, tc.want)
			}
		})
	}
}

func TestGetJSONTimeout(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}, Config{Timeout: 50 * time.Millisecond})
	var out map[string]any
	err := c.GetJSON(context.Background(), c.URL("/x", nil), &out)
	if !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("erro = %v", err)
	}
}

func TestGetJSONRetriesOnceOn429(t *testing.T) {
	var calls atomic.Int32
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}, Config{})
	var out map[string]any
	if err := c.GetJSON(context.Background(), c.URL("/x", nil), &out); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("chamadas = %d", calls.Load())
	}

	// Sempre 429: só uma nova tentativa, depois ErrUnavailable.
	calls.Store(0)
	c = newClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}, Config{})
	if err := c.GetJSON(context.Background(), c.URL("/x", nil), &out); !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("erro = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("chamadas = %d", calls.Load())
	}
}

func TestConcurrencyLimit(t *testing.T) {
	var inFlight, peak atomic.Int32
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)
		_, _ = w.Write([]byte(`{}`))
	}, Config{Concurrency: 2})
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			var out map[string]any
			if err := c.GetJSON(context.Background(), c.URL("/x", nil), &out); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if p := peak.Load(); p != 2 {
		t.Fatalf("pico de requisições simultâneas = %d, quero 2", p)
	}
}

func TestDate(t *testing.T) {
	cases := map[string]string{
		"2019-12-11T17:37":          "2019-12-11T17:37:00-03:00",
		"2026-05-28 18:42:02":       "2026-05-28T18:42:02-03:00",
		"2026-05-28":                "2026-05-28T00:00:00-03:00",
		"2026-11-03T13:54:25.456":   "2026-11-03T13:54:25.456-03:00",
		"2020-01-01T10:00:00Z":      "2020-01-01T10:00:00Z",
		"2020-01-01T10:00:00+01:00": "2020-01-01T10:00:00+01:00",
	}
	for in, want := range cases {
		got, err := Date("d", in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		w, _ := time.Parse(time.RFC3339Nano, want)
		if !got.Equal(w) {
			t.Fatalf("%q = %v, quero %v", in, got, w)
		}
	}
	for _, bad := range []string{"", "ontem", "1800-01-01", "2999-01-01", "2019-13-40"} {
		if _, err := Date("d", bad); !errors.Is(err, app.ErrUnavailable) {
			t.Fatalf("%q: erro = %v", bad, err)
		}
	}
}

func TestNormalizeQueryAndIDs(t *testing.T) {
	q, err := NormalizeQuery(app.SearchQuery{Kind: " pec ", Number: 221, Year: 2019})
	if err != nil || q.Kind != "PEC" {
		t.Fatalf("q = %+v, %v", q, err)
	}
	for _, bad := range []app.SearchQuery{
		{Kind: "", Number: 1, Year: 2019},
		{Kind: "PL/../x", Number: 1, Year: 2019},
		{Kind: "PL", Number: 0, Year: 2019},
		{Kind: "PL", Number: 1, Year: 19},
	} {
		if _, err := NormalizeQuery(bad); !errors.Is(err, app.ErrNotFound) {
			t.Fatalf("%+v: erro = %v", bad, err)
		}
	}
	for _, bad := range []string{"", "0", "abc", "1/../2", "12345678901234567", "1?x=2"} {
		if _, err := ExternalID(bad); !errors.Is(err, app.ErrNotFound) {
			t.Fatalf("%q: erro = %v", bad, err)
		}
	}
}

func TestTextAndURL(t *testing.T) {
	got, err := Line("x", "  Altera\r\n o \x00art.\t7º  ")
	if err != nil || got != "Altera o art. 7º" {
		t.Fatalf("Line = %q, %v", got, err)
	}
	if _, err := Text("x", strings.Repeat("a", MaxTextLen+1)); !errors.Is(err, app.ErrUnavailable) {
		t.Fatalf("texto enorme: %v", err)
	}
	for in, want := range map[string]string{
		"https://www.camara.leg.br/x?codteor=1":            "https://www.camara.leg.br/x?codteor=1",
		"https://legis.senado.gov.br/sdleg-getter/doc/abc": "https://legis.senado.gov.br/sdleg-getter/doc/abc",
		"javascript:alert(1)":                              "",
		"https://evil.example/x.leg.br":                    "",
		"https://u:p@www.camara.leg.br/":                   "",
		"":                                                 "",
	} {
		if got := OfficialURL(in); got != want {
			t.Fatalf("OfficialURL(%q) = %q, quero %q", in, got, want)
		}
	}
}
