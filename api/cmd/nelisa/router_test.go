package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/platform/config"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func serve(p Pinger, method, path string) *httptest.ResponseRecorder {
	h := newRouter(config.Config{}, p, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestHealthz(t *testing.T) {
	if w := serve(fakePinger{}, "GET", "/healthz"); w.Code != http.StatusOK {
		t.Errorf("healthy db: status %d", w.Code)
	}
	if w := serve(fakePinger{errors.New("down")}, "GET", "/healthz"); w.Code != http.StatusServiceUnavailable {
		t.Errorf("db down: status %d", w.Code)
	}
}

func TestVersion(t *testing.T) {
	w := serve(fakePinger{}, "GET", "/version")
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["commit"] == "" {
		t.Errorf("unexpected /version body: %s", w.Body)
	}
}

func TestUnknownRouteUsesStandardError(t *testing.T) {
	w := serve(fakePinger{}, "GET", "/api/public/nope")
	var e httpx.ErrorBody
	if w.Code != http.StatusNotFound || json.Unmarshal(w.Body.Bytes(), &e) != nil || e.Error.Code != "not_found" {
		t.Errorf("got %d %s", w.Code, w.Body)
	}
}
