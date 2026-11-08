// Package httpapi é o adaptador HTTP do módulo queue: a Fila de revisão e
// assumir/soltar a Posse, sob /api/team (Editor ou acima).
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
	"github.com/mphennrichs/nelisa/api/internal/queue/app"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
)

// UseCases reúne os casos de uso que o HTTP dirige.
type UseCases struct {
	Claim     *app.Claim
	Release   *app.Release
	ListQueue *app.ListQueue
}

// Handlers são os handlers HTTP do módulo.
type Handlers struct {
	uc     UseCases
	logger *slog.Logger
}

func New(uc UseCases, logger *slog.Logger) *Handlers { return &Handlers{uc: uc, logger: logger} }

// RegisterTeam monta as rotas em r (o roteador de /api/team, já atrás da
// autenticação).
func (h *Handlers) RegisterTeam(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireRole(auth.RoleEditor))
		r.Get("/queue", h.list)
		r.Post("/propositions/{id}/claim", h.claim)
		r.Delete("/propositions/{id}/claim", h.release)
	})
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

type claimDTO struct {
	HolderID       string `json:"holder_id"`
	HolderName     string `json:"holder_name"`
	ClaimedAt      string `json:"claimed_at"`
	LastActivityAt string `json:"last_activity_at"`
	ExpiresAt      string `json:"expires_at"`
}

func toClaimDTO(c *app.ClaimView) *claimDTO {
	if c == nil {
		return nil
	}
	return &claimDTO{
		HolderID: c.UserID, HolderName: c.HolderName, ClaimedAt: ts(c.ClaimedAt),
		LastActivityAt: ts(c.LastActivityAt), ExpiresAt: ts(c.ExpiresAt()),
	}
}

type versionDTO struct {
	ID    string `json:"id"`
	Seq   int    `json:"seq"`
	Label string `json:"label"`
}

type missingDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type itemDTO struct {
	PropositionID string       `json:"proposition_id"`
	Name          string       `json:"name"`
	Slug          string       `json:"slug"`
	Sphere        string       `json:"sphere"`
	Reason        string       `json:"reason"`
	Version       versionDTO   `json:"version"`
	Since         string       `json:"since"`
	Claim         *claimDTO    `json:"claim"`
	Missing       []missingDTO `json:"missing"`
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.uc.ListQueue.Execute(r.Context(), app.ListQueueInput{Sphere: r.URL.Query().Get("sphere")})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	out := make([]itemDTO, 0, len(items))
	for _, it := range items {
		d := itemDTO{
			PropositionID: it.PropositionID, Name: it.Name, Slug: it.Slug, Sphere: string(it.Sphere),
			Reason: string(it.Reason), Since: ts(it.Since), Claim: toClaimDTO(it.Claim),
			Version: versionDTO{ID: it.Version.ID, Seq: it.Version.Seq, Label: it.Version.Label},
			Missing: make([]missingDTO, 0, len(it.Missing)),
		}
		for _, m := range it.Missing {
			d.Missing = append(d.Missing, missingDTO(m))
		}
		out = append(out, d)
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handlers) claim(w http.ResponseWriter, r *http.Request) {
	c, err := h.uc.Claim.Execute(r.Context(), app.ClaimInput{PropositionID: chi.URLParam(r, "id")})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toClaimDTO(&c))
}

func (h *Handlers) release(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.Release.Execute(r.Context(), app.ReleaseInput{PropositionID: chi.URLParam(r, "id")}); err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type errMapping struct {
	err     error
	status  int
	code    string
	message string
}

func errMappings() []errMapping {
	return []errMapping{
		{app.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated", "Faça login para continuar."},
		{app.ErrForbidden, http.StatusForbidden, "forbidden", "Você não tem permissão para isso."},
		{domain.ErrNotHolder, http.StatusForbidden, "forbidden", "Só quem tem a posse, ou Revisor/Admin, pode soltá-la."},
		{app.ErrPropositionNotFound, http.StatusNotFound, "proposition_not_found", "Proposição não encontrada."},
		{domain.ErrInvalidSphere, http.StatusBadRequest, "invalid_sphere", "Esfera inválida (federal, state ou municipal)."},
	}
}

func writeErr(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var taken *domain.ClaimTakenError
	if errors.As(err, &taken) {
		httpx.WriteError(w, http.StatusConflict, "claim_taken", claimTakenMessage(taken.HolderName))
		return
	}
	for _, m := range errMappings() {
		if errors.Is(err, m.err) {
			httpx.WriteError(w, m.status, m.code, m.message)
			return
		}
	}
	logger.ErrorContext(r.Context(), "queue request failed", "error", err.Error())
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Erro interno. Tente de novo.")
}

// claimTakenMessage é a mensagem do 409 claim_taken, com o nome de quem
// tem a Posse (só a equipe a vê).
func claimTakenMessage(holder string) string {
	if holder == "" {
		holder = "Outra pessoa"
	}
	return holder + " está com a posse desta Proposição."
}
