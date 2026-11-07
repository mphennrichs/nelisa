// Package observability tem o logger estruturado e o log de requisições.
package observability

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewLogger devolve um logger JSON no stdout.
func NewLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// quietPaths são consultados por healthcheck e não ajudam a diagnosticar.
var quietPaths = map[string]bool{"/healthz": true}

// RequestLogger registra uma linha por requisição. Usa o padrão da rota do
// chi, nunca a URL crua (que carrega IDs), e nunca registra corpo, cookies
// ou cabeçalhos de autorização.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if quietPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			defer func() {
				route := "unmatched"
				if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
					route = rc.RoutePattern()
				}
				level := slog.LevelInfo
				if ww.Status() >= 500 {
					level = slog.LevelError
				}
				logger.Log(r.Context(), level, "http request",
					"method", r.Method,
					"route", route,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"duration_ms", time.Since(start).Milliseconds(),
					"request_id", middleware.GetReqID(r.Context()),
				)
			}()
			next.ServeHTTP(ww, r)
		})
	}
}
