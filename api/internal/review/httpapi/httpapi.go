// Package httpapi é o adaptador HTTP do módulo review: aprovar a Versão em
// análise e confirmar a segunda revisão, sob /api/team (Revisor ou acima).
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
	"github.com/mphennrichs/nelisa/api/internal/review/app"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

// UseCases reúne os casos de uso que o HTTP dirige.
type UseCases struct {
	Approve      *app.Approve
	SecondReview *app.SecondReview
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
		// Recusa cedo; os casos de uso conferem de novo.
		r.Use(auth.RequireRole(auth.RoleReviewer))
		r.Post("/propositions/{id}/versions/{versionId}/approve", h.approve)
		r.Post("/propositions/{id}/versions/{versionId}/second-review", h.secondReview)
	})
}

type approvalDTO struct {
	VersionID           string  `json:"version_id"`
	ApprovedAt          string  `json:"approved_at"`
	FourEyes            bool    `json:"four_eyes"`
	SecondReviewAt      *string `json:"second_review_at"`
	PendingSecondReview bool    `json:"pending_second_review"`
	ReviewBadge         int     `json:"review_badge"`
}

func toDTO(a domain.Approval) approvalDTO {
	out := approvalDTO{
		VersionID: a.VersionID, ApprovedAt: a.ApprovedAt.UTC().Format(time.RFC3339), FourEyes: a.FourEyes,
		PendingSecondReview: a.PendingSecondReview(), ReviewBadge: a.Badge(),
	}
	if a.SecondReviewAt != nil {
		s := a.SecondReviewAt.UTC().Format(time.RFC3339)
		out.SecondReviewAt = &s
	}
	return out
}

func (h *Handlers) approve(w http.ResponseWriter, r *http.Request) {
	a, err := h.uc.Approve.Execute(r.Context(), app.ApproveInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"),
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(a))
}

func (h *Handlers) secondReview(w http.ResponseWriter, r *http.Request) {
	a, err := h.uc.SecondReview.Execute(r.Context(), app.SecondReviewInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"),
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(a))
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
		{domain.ErrNotReviewer, http.StatusForbidden, "forbidden", "Você não tem permissão para isso."},
		{domain.ErrFourEyes, http.StatusForbidden, "four_eyes", "Quem escreveu ou aprovou este conteúdo não pode revisá-lo: outra pessoa precisa fazer isso (quatro olhos)."},
		{app.ErrPropositionNotFound, http.StatusNotFound, "proposition_not_found", "Proposição não encontrada."},
		{app.ErrVersionNotFound, http.StatusNotFound, "version_not_found", "Versão não encontrada."},
		{app.ErrVersionNotPending, http.StatusConflict, "version_not_pending", "Esta Versão não está mais em análise."},
		{domain.ErrNoPendingVersion, http.StatusConflict, "version_not_pending", "Esta Versão não está mais em análise."},
		{domain.ErrAlreadyApproved, http.StatusConflict, "version_not_pending", "Esta Versão já foi aprovada."},
		{domain.ErrNotInReview, http.StatusConflict, "invalid_review_transition", "Envie a Proposição para revisão antes de aprovar."},
		{domain.ErrNoSecondReviewPending, http.StatusConflict, "no_second_review_pending", "Não há segunda revisão pendente para esta Versão."},
		{domain.ErrApprovalNotFound, http.StatusConflict, "no_second_review_pending", "Não há segunda revisão pendente para esta Versão."},
	}
}

func writeErr(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var incomplete *domain.ContentIncompleteError
	if errors.As(err, &incomplete) {
		msgs := make([]string, 0, len(incomplete.Missing))
		for _, m := range incomplete.Missing {
			msgs = append(msgs, m.Message)
		}
		httpx.WriteError(w, http.StatusConflict, "content_incomplete", "Falta para aprovar: "+strings.Join(msgs, " "))
		return
	}
	for _, m := range errMappings() {
		if errors.Is(err, m.err) {
			httpx.WriteError(w, m.status, m.code, m.message)
			return
		}
	}
	logger.ErrorContext(r.Context(), "review request failed", "error", err.Error())
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Erro interno. Tente de novo.")
}
