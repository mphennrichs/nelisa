// Package httpapi é o adaptador HTTP do módulo review, sob /api/team:
// aprovar a Versão em análise e confirmar a segunda revisão (Revisor ou
// acima); Correção menor e Errata (propor e retirar: Editor ou acima;
// conferir, aprovar e recusar: Revisor ou acima).
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
	Approve         *app.Approve
	SecondReview    *app.SecondReview
	ApplyMinorFix   *app.ApplyMinorFix
	CheckMinorFix   *app.CheckMinorFix
	ProposeErratum  *app.ProposeErratum
	ApproveErratum  *app.ApproveErratum
	RejectErratum   *app.RejectErratum
	WithdrawErratum *app.WithdrawErratum
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
	const errata = "/propositions/{id}/versions/{versionId}/errata"
	r.Group(func(r chi.Router) {
		// Recusa cedo; os casos de uso conferem de novo.
		r.Use(auth.RequireRole(auth.RoleEditor))
		r.Put("/propositions/{id}/versions/{versionId}/minor-fix", h.minorFix)
		r.Post(errata, h.proposeErratum)
		r.Post(errata+"/{erratumId}/withdraw", h.withdrawErratum)
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireRole(auth.RoleReviewer))
		r.Post("/propositions/{id}/versions/{versionId}/approve", h.approve)
		r.Post("/propositions/{id}/versions/{versionId}/second-review", h.secondReview)
		r.Post("/propositions/{id}/minor-fixes/{fixId}/check", h.checkMinorFix)
		r.Post(errata+"/{erratumId}/approve", h.approveErratum)
		r.Post(errata+"/{erratumId}/reject", h.rejectErratum)
	})
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func tsPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := ts(*t)
	return &s
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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
	return approvalDTO{
		VersionID: a.VersionID, ApprovedAt: ts(a.ApprovedAt), FourEyes: a.FourEyes,
		PendingSecondReview: a.PendingSecondReview(), ReviewBadge: a.Badge(), SecondReviewAt: tsPtr(a.SecondReviewAt),
	}
}

func (h *Handlers) approve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Revision *int `json:"revision"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Revision == nil {
		httpx.WriteError(w, http.StatusBadRequest, "revision_required", "Informe a revision do conteúdo que você leu.")
		return
	}
	a, err := h.uc.Approve.Execute(r.Context(), app.ApproveInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"), Revision: *req.Revision,
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
		{domain.ErrStaleContent, http.StatusConflict, "stale_content", "O conteúdo mudou desde que você o leu. Recarregue e confira antes de continuar."},
		{domain.ErrAlreadyApproved, http.StatusConflict, "version_not_pending", "Esta Versão já foi aprovada."},
		{domain.ErrNotInReview, http.StatusConflict, "invalid_review_transition", "Envie a Proposição para revisão antes de aprovar."},
		{domain.ErrNoSecondReviewPending, http.StatusConflict, "no_second_review_pending", "Não há segunda revisão pendente para esta Versão."},
		{domain.ErrApprovalNotFound, http.StatusConflict, "no_second_review_pending", "Não há segunda revisão pendente para esta Versão."},
		{domain.ErrVersionNotCurrent, http.StatusConflict, "version_not_current", "Só a Versão vigente recebe Correção menor ou Errata."},
		{domain.ErrNotReady, http.StatusConflict, "not_ready", "Correção menor e Errata são só para Proposição pronta (sem Versão em análise)."},
		{domain.ErrErratumPending, http.StatusConflict, "erratum_pending", "Há uma Errata pendente nesta Versão: decida-a antes de corrigir o texto de novo."},
		{domain.ErrNoChanges, http.StatusConflict, "no_changes", "O texto enviado é igual ao publicado: não há o que corrigir."},
		{domain.ErrInvalidErratumNote, http.StatusBadRequest, "invalid_erratum_note", "Preencha \"Dizia\" e \"O correto é\" (até 500 caracteres cada)."},
		{domain.ErrInvalidReason, http.StatusBadRequest, "invalid_reason", "Informe o motivo da recusa (até 1.000 caracteres)."},
		{domain.ErrMinorFixNotFound, http.StatusNotFound, "minor_fix_not_found", "Correção menor não encontrada."},
		{domain.ErrMinorFixChecked, http.StatusConflict, "minor_fix_checked", "Esta Correção menor já foi conferida."},
		{domain.ErrErratumNotFound, http.StatusNotFound, "erratum_not_found", "Errata não encontrada."},
		{domain.ErrErratumNotPending, http.StatusConflict, "erratum_not_pending", "Esta Errata já foi decidida ou retirada."},
		{domain.ErrNotErratumProposer, http.StatusForbidden, "forbidden", "Só quem propôs a Errata pode retirá-la."},
	}
}

func writeErr(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var taken *app.ClaimTakenError
	if errors.As(err, &taken) {
		holder := taken.HolderName
		if holder == "" {
			holder = "Outra pessoa"
		}
		httpx.WriteError(w, http.StatusConflict, "claim_taken", holder+" está com a posse desta Proposição.")
		return
	}
	var invalid *app.InvalidContentError
	if errors.As(err, &invalid) {
		httpx.WriteError(w, http.StatusBadRequest, invalid.Code, invalid.Message)
		return
	}
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
