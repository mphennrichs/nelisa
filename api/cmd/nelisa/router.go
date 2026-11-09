package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/mphennrichs/nelisa/api/internal/platform/buildinfo"
	"github.com/mphennrichs/nelisa/api/internal/platform/config"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
	"github.com/mphennrichs/nelisa/api/internal/platform/observability"
)

// Pinger é o que o /healthz precisa do banco.
type Pinger interface {
	Ping(ctx context.Context) error
}

// newRouter monta as rotas. Rotas públicas ficam em /api/public (sempre
// filtradas pela regra de visibilidade); rotas da equipe em /api/team
// (sempre autenticadas). Os módulos se registram aqui conforme chegam.
func newRouter(_ config.Config, db Pinger, team teamModule, content contentModule, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.NotFound(httpx.NotFound)
	r.MethodNotAllowed(httpx.MethodNotAllowed)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(httpx.LimitBody(httpx.MaxBodyBytes))
	r.Use(observability.RequestLogger(logger))

	r.Get("/version", buildinfo.Handler())
	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "database_unreachable", "Banco indisponível.")
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	r.Route("/api/public", func(r chi.Router) {
		// Só a camada pública do proposition (regra de visibilidade).
		content.prop.RegisterPublic(r)
		// Contribuições do Leitor: não leem conteúdo da equipe; a consulta
		// devolve só tipo, estado e motivo público.
		content.contrib.RegisterPublic(r)
	})
	r.Route("/api/team", func(r chi.Router) {
		// /auth é a única parte de /api/team sem login (bootstrap, login,
		// refresh, logout); todo o resto passa pela autenticação.
		team.handlers.RegisterAuth(r)
		r.Group(func(r chi.Router) {
			r.Use(team.authenticate)
			team.handlers.RegisterAuthenticated(r)
			content.prop.RegisterTeam(r)
			content.review.RegisterTeam(r)
			content.queue.RegisterTeam(r)
			content.ingest.RegisterTeam(r)
			content.contrib.RegisterTeam(r)
		})
	})
	return r
}
