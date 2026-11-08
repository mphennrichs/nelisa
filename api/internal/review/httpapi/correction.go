package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
	"github.com/mphennrichs/nelisa/api/internal/review/app"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

// Correção menor e Errata, só para a equipe. Os ids de pessoas vão crus:
// os nomes vêm no detalhe da Proposição.

type snapshotDTO struct {
	Summary      string   `json:"summary"`
	ShortSummary string   `json:"short_summary"`
	TopicIDs     []string `json:"topic_ids"`
}

type minorFixDTO struct {
	ID        string      `json:"id"`
	VersionID string      `json:"version_id"`
	Fields    []string    `json:"fields"`
	Before    snapshotDTO `json:"before"`
	After     snapshotDTO `json:"after"`
	CreatedBy string      `json:"created_by"`
	CreatedAt string      `json:"created_at"`
	CheckedBy *string     `json:"checked_by"`
	CheckedAt *string     `json:"checked_at"`
}

type erratumDTO struct {
	ID           string      `json:"id"`
	VersionID    string      `json:"version_id"`
	Proposed     snapshotDTO `json:"proposed"`
	Said         string      `json:"said"`
	Correct      string      `json:"correct"`
	BaseRevision int         `json:"base_revision"`
	State        string      `json:"state"`
	CreatedBy    string      `json:"created_by"`
	CreatedAt    string      `json:"created_at"`
	DecidedBy    *string     `json:"decided_by"`
	DecidedAt    *string     `json:"decided_at"`
	Reason       string      `json:"reason"`
}

func toSnapshotDTO(s domain.Snapshot) snapshotDTO {
	topics := s.TopicIDs
	if topics == nil {
		topics = []string{}
	}
	return snapshotDTO{Summary: s.Summary, ShortSummary: s.ShortSummary, TopicIDs: topics}
}

func toMinorFixDTO(f domain.MinorFix) minorFixDTO {
	return minorFixDTO{
		ID: f.ID, VersionID: f.VersionID, Fields: f.Fields, Before: toSnapshotDTO(f.Before), After: toSnapshotDTO(f.After),
		CreatedBy: f.CreatedBy, CreatedAt: ts(f.CreatedAt), CheckedBy: optStr(f.CheckedBy), CheckedAt: tsPtr(f.CheckedAt),
	}
}

func toErratumDTO(e domain.Erratum) erratumDTO {
	return erratumDTO{
		ID: e.ID, VersionID: e.VersionID, Proposed: toSnapshotDTO(e.Proposed), Said: e.Said, Correct: e.Correct,
		BaseRevision: e.BaseRevision, State: string(e.State), CreatedBy: e.CreatedBy, CreatedAt: ts(e.CreatedAt),
		DecidedBy: optStr(e.DecidedBy), DecidedAt: tsPtr(e.DecidedAt), Reason: e.Reason,
	}
}

type contentRequest struct {
	Summary      string   `json:"summary"`
	ShortSummary string   `json:"short_summary"`
	TopicIDs     []string `json:"topic_ids"`
	Revision     *int     `json:"revision"`
}

func revisionRequired(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "revision_required", "Informe a revision do conteúdo que você leu.")
}

func (h *Handlers) minorFix(w http.ResponseWriter, r *http.Request) {
	var req contentRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Revision == nil {
		revisionRequired(w)
		return
	}
	f, err := h.uc.ApplyMinorFix.Execute(r.Context(), app.ApplyMinorFixInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"),
		Summary: req.Summary, ShortSummary: req.ShortSummary, TopicIDs: req.TopicIDs, Revision: *req.Revision,
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMinorFixDTO(f))
}

func (h *Handlers) checkMinorFix(w http.ResponseWriter, r *http.Request) {
	f, err := h.uc.CheckMinorFix.Execute(r.Context(), app.CheckMinorFixInput{
		PropositionID: chi.URLParam(r, "id"), MinorFixID: chi.URLParam(r, "fixId"),
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMinorFixDTO(f))
}

type proposeErratumRequest struct {
	contentRequest
	Said    string `json:"said"`
	Correct string `json:"correct"`
}

func (h *Handlers) proposeErratum(w http.ResponseWriter, r *http.Request) {
	var req proposeErratumRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Revision == nil {
		revisionRequired(w)
		return
	}
	e, err := h.uc.ProposeErratum.Execute(r.Context(), app.ProposeErratumInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"),
		Summary: req.Summary, ShortSummary: req.ShortSummary, TopicIDs: req.TopicIDs,
		Said: req.Said, Correct: req.Correct, Revision: *req.Revision,
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toErratumDTO(e))
}

func erratumRef(r *http.Request) app.ErratumRef {
	return app.ErratumRef{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"), ErratumID: chi.URLParam(r, "erratumId"),
	}
}

func (h *Handlers) approveErratum(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Revision *int `json:"revision"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Revision == nil {
		revisionRequired(w)
		return
	}
	e, err := h.uc.ApproveErratum.Execute(r.Context(), app.ApproveErratumInput{ErratumRef: erratumRef(r), Revision: *req.Revision})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toErratumDTO(e))
}

func (h *Handlers) rejectErratum(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	e, err := h.uc.RejectErratum.Execute(r.Context(), app.RejectErratumInput{ErratumRef: erratumRef(r), Reason: req.Reason})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toErratumDTO(e))
}

func (h *Handlers) withdrawErratum(w http.ResponseWriter, r *http.Request) {
	e, err := h.uc.WithdrawErratum.Execute(r.Context(), erratumRef(r))
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toErratumDTO(e))
}
