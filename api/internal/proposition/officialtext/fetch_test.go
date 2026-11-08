package officialtext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// routes responde sem rede: cada URL completa tem um handler.
type routes map[string]http.HandlerFunc

func (r routes) RoundTrip(req *http.Request) (*http.Response, error) {
	h, ok := r[req.URL.String()]
	if !ok {
		return nil, fmt.Errorf("sem rota para %s (a busca saiu da lista?)", req.URL)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec.Result(), nil
}

type pagesOf []string

func (p pagesOf) Pages(context.Context, []byte) ([]string, error) { return p, nil }

type brokenPDF struct{}

func (brokenPDF) Pages(context.Context, []byte) ([]string, error) {
	return nil, errors.New("xref quebrada")
}

const official = "https://www.camara.leg.br/proposicoesWeb/prop_mostrarintegra?codteor=1"

func pdf(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = io.WriteString(w, "%PDF-1.4\n...")
}

func failure(t *testing.T, err error) (domain.StructuringFailure, bool) {
	t.Helper()
	var fe *app.TextFetchError
	if !errors.As(err, &fe) {
		t.Fatalf("erro sem motivo: %v", err)
	}
	return fe.Failure, fe.Temporary
}

func TestFetchReadsOfficialPDF(t *testing.T) {
	var gotUA string
	f := New(pagesOf{"página 1", "página 2"}, routes{official: func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		pdf(w, r)
	}})
	pages, err := f.Fetch(context.Background(), official)
	if err != nil || len(pages) != 2 {
		t.Fatalf("pages %v err %v", pages, err)
	}
	if !strings.HasPrefix(gotUA, "Nelisa/") {
		t.Errorf("user-agent %q", gotUA)
	}
}

func TestFetchFollowsRedirectOnlyInsideOfficialSources(t *testing.T) {
	senado := "https://legis.senado.leg.br/sdleg-getter/documento?dm=1"
	ok := New(pagesOf{"x"}, routes{
		official: func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, senado, http.StatusFound) },
		senado:   pdf,
	})
	if _, err := ok.Fetch(context.Background(), official); err != nil {
		t.Fatalf("redirecionamento dentro das fontes: %v", err)
	}

	evil := New(pagesOf{"x"}, routes{
		official: func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://169.254.169.254/latest/meta-data", http.StatusFound)
		},
	})
	_, err := evil.Fetch(context.Background(), official)
	if f, temp := failure(t, err); f != domain.FailureHostNotAllowed || temp {
		t.Fatalf("redirecionamento para fora: %s temp=%v", f, temp)
	}
}

func TestFetchRefusesURLOutsideOfficialSources(t *testing.T) {
	f := New(pagesOf{"x"}, routes{})
	for _, u := range []string{"https://example.com/a.pdf", "http://www.camara.leg.br/a.pdf", "file:///etc/passwd"} {
		_, err := f.Fetch(context.Background(), u)
		if got, _ := failure(t, err); got != domain.FailureHostNotAllowed {
			t.Errorf("%s: %s", u, got)
		}
	}
}

func TestFetchFailures(t *testing.T) {
	big := bytes.Repeat([]byte("a"), MaxPDFBytes+1)
	copy(big, "%PDF-")
	for _, tt := range []struct {
		name      string
		h         http.HandlerFunc
		ex        Extractor
		want      domain.StructuringFailure
		temporary bool
	}{
		{"404", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }, pagesOf{}, domain.FailureNotFound, false},
		{"503", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }, pagesOf{}, domain.FailureNetwork, true},
		{"429", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(429) }, pagesOf{}, domain.FailureNetwork, true},
		{"página html", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "<html>erro</html>") },
			pagesOf{}, domain.FailureNotPDF, false},
		{"grande demais", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(big) }, pagesOf{}, domain.FailureTooLarge, false},
		{"pdf quebrado", pdf, brokenPDF{}, domain.FailureNotPDF, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.ex, routes{official: tt.h}).Fetch(context.Background(), official)
			if f, temp := failure(t, err); f != tt.want || temp != tt.temporary {
				t.Fatalf("%s temp=%v, quer %s temp=%v", f, temp, tt.want, tt.temporary)
			}
		})
	}
}

func TestFetchNetworkErrorIsTemporary(t *testing.T) {
	_, err := New(pagesOf{}, routes{}).Fetch(context.Background(), official) // sem rota: erro de transporte
	if f, temp := failure(t, err); f != domain.FailureNetwork || !temp {
		t.Fatalf("%s temp=%v", f, temp)
	}
}
