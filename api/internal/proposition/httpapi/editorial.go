package httpapi

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// DTOs do ciclo editorial, só para a equipe. Ids de pessoas vêm com os
// nomes no mapa people.

type versionDTO struct {
	ID              string  `json:"id"`
	Seq             int     `json:"seq"`
	Label           string  `json:"label"`
	OfficialTextURL string  `json:"official_text_url"`
	State           string  `json:"state"`
	CreatedBy       *string `json:"created_by"`
	CreatedAt       string  `json:"created_at"`
}

type contentDTO struct {
	VersionID         string   `json:"version_id"`
	Summary           string   `json:"summary"`
	ShortSummary      string   `json:"short_summary"`
	TopicIDs          []string `json:"topic_ids"`
	NeedsConfirmation bool     `json:"needs_confirmation"`
	Authors           []string `json:"authors"`
	Revision          int      `json:"revision"`
	UpdatedBy         *string  `json:"updated_by"`
	UpdatedAt         string   `json:"updated_at"`
}

type topicDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

type teamClaimDTO struct {
	HolderID       string `json:"holder_id"`
	HolderName     string `json:"holder_name"`
	ClaimedAt      string `json:"claimed_at"`
	LastActivityAt string `json:"last_activity_at"`
	ExpiresAt      string `json:"expires_at"`
	Mine           bool   `json:"mine"`
	CanRelease     bool   `json:"can_release"`
}

type approvalInfoDTO struct {
	VersionID           string  `json:"version_id"`
	ApprovedBy          string  `json:"approved_by"`
	ApprovedAt          string  `json:"approved_at"`
	FourEyes            bool    `json:"four_eyes"`
	SecondReviewBy      *string `json:"second_review_by"`
	SecondReviewAt      *string `json:"second_review_at"`
	PendingSecondReview bool    `json:"pending_second_review"`
}

type blockerDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type editorialDTO struct {
	Versions               []versionDTO      `json:"versions"`
	PendingContent         *contentDTO       `json:"pending_content"`
	CurrentContent         *contentDTO       `json:"current_content"`
	Topics                 []topicDTO        `json:"topics"`
	Claim                  *teamClaimDTO     `json:"claim"`
	ReviewBadge            *int              `json:"review_badge"`
	CurrentApproval        *approvalInfoDTO  `json:"current_approval"`
	CanEdit                bool              `json:"can_edit"`
	CanCreateVersion       bool              `json:"can_create_version"`
	CanApprove             bool              `json:"can_approve"`
	ApproveBlockers        []blockerDTO      `json:"approve_blockers"`
	ApprovalBreaksFourEyes bool              `json:"approval_breaks_four_eyes"`
	CanSecondReview        bool              `json:"can_second_review"`
	CanMinorFix            bool              `json:"can_minor_fix"`
	CanProposeErratum      bool              `json:"can_propose_erratum"`
	PendingStructuring     *structuringDTO   `json:"pending_structuring"`
	CurrentStructuring     *structuringDTO   `json:"current_structuring"`
	CanEditStructuring     bool              `json:"can_edit_structuring"`
	CanRetryStructuring    bool              `json:"can_retry_structuring"`
	MinorFixes             []minorFixDTO     `json:"minor_fixes"`
	PendingErratum         *erratumDTO       `json:"pending_erratum"`
	Errata                 []erratumDTO      `json:"errata"`
	People                 map[string]string `json:"people"`
}

// structuringDTO é a Estruturação de uma Versão para a equipe: o texto
// que gerou os Dispositivos, os avisos do parser e a árvore.
type structuringDTO struct {
	VersionID   string             `json:"version_id"`
	State       string             `json:"state"`
	Failure     *string            `json:"failure"`
	SourceText  string             `json:"source_text"`
	Warnings    []structWarningDTO `json:"warnings"`
	Provisions  []teamProvisionDTO `json:"provisions"`
	Revision    int                `json:"revision"`
	UpdatedBy   *string            `json:"updated_by"`
	UpdatedAt   string             `json:"updated_at"`
	ConfirmedBy *string            `json:"confirmed_by"`
	ConfirmedAt *string            `json:"confirmed_at"`
}

type structWarningDTO struct {
	Code    string  `json:"code"`
	Line    int     `json:"line"`
	Address *string `json:"address"`
}

type teamProvisionDTO struct {
	Address     string `json:"address"`
	Parent      string `json:"parent"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Text        string `json:"text"`
	Depth       int    `json:"depth"`
	Commentable bool   `json:"commentable"`
}

func toStructuringDTO(v app.StructuringView) structuringDTO {
	out := structuringDTO{
		VersionID: v.VersionID, State: string(v.State), SourceText: v.SourceText,
		Warnings: make([]structWarningDTO, 0, len(v.Warnings)), Provisions: make([]teamProvisionDTO, 0, len(v.Provisions)),
		Revision: v.Revision, UpdatedBy: strPtr(v.UpdatedBy), UpdatedAt: ts(v.UpdatedAt),
		ConfirmedBy: strPtr(v.ConfirmedBy), ConfirmedAt: tsPtr(v.ConfirmedAt),
	}
	if v.Failure != domain.FailureNone {
		f := string(v.Failure)
		out.Failure = &f
	}
	for _, w := range v.Warnings {
		out.Warnings = append(out.Warnings, structWarningDTO{Code: w.Code, Line: w.Line, Address: strPtr(w.Address)})
	}
	for _, p := range v.Provisions {
		out.Provisions = append(out.Provisions, teamProvisionDTO{
			Address: p.Address, Parent: p.Parent, Kind: string(p.Kind), Label: p.Label, Text: p.Text, Depth: p.Depth,
			Commentable: p.Kind.Commentable(),
		})
	}
	return out
}

type snapshotDTO struct {
	Summary      string   `json:"summary"`
	ShortSummary string   `json:"short_summary"`
	TopicIDs     []string `json:"topic_ids"`
}

// minorFixDTO é uma Correção menor a conferir, com o antes e o depois.
type minorFixDTO struct {
	ID               string      `json:"id"`
	VersionID        string      `json:"version_id"`
	Fields           []string    `json:"fields"`
	Before           snapshotDTO `json:"before"`
	After            snapshotDTO `json:"after"`
	CreatedBy        string      `json:"created_by"`
	CreatedAt        string      `json:"created_at"`
	CanCheckMinorFix bool        `json:"can_check_minor_fix"`
}

// erratumDTO é uma Errata para a equipe (o Leitor só vê data e nota).
type erratumDTO struct {
	ID                 string      `json:"id"`
	VersionID          string      `json:"version_id"`
	Proposed           snapshotDTO `json:"proposed"`
	Said               string      `json:"said"`
	Correct            string      `json:"correct"`
	BaseRevision       int         `json:"base_revision"`
	State              string      `json:"state"`
	CreatedBy          string      `json:"created_by"`
	CreatedAt          string      `json:"created_at"`
	DecidedBy          *string     `json:"decided_by"`
	DecidedAt          *string     `json:"decided_at"`
	Reason             string      `json:"reason"`
	Stale              bool        `json:"stale"`
	CanDecideErratum   bool        `json:"can_decide_erratum"`
	CanWithdrawErratum bool        `json:"can_withdraw_erratum"`
}

func toSnapshotDTO(s app.ContentSnapshot) snapshotDTO {
	return snapshotDTO{Summary: s.Summary, ShortSummary: s.ShortSummary, TopicIDs: nonNil(s.TopicIDs)}
}

func toErratumDTO(e app.ErratumInfo) erratumDTO {
	return erratumDTO{
		ID: e.ID, VersionID: e.VersionID, Proposed: toSnapshotDTO(e.Proposed), Said: e.Said, Correct: e.Correct,
		BaseRevision: e.BaseRevision, State: e.State, CreatedBy: e.CreatedBy, CreatedAt: ts(e.CreatedAt),
		DecidedBy: strPtr(e.DecidedBy), DecidedAt: tsPtr(e.DecidedAt), Reason: e.Reason, Stale: e.Stale,
		CanDecideErratum: e.CanDecide, CanWithdrawErratum: e.CanWithdraw,
	}
}

func toVersionDTO(v domain.Version) versionDTO {
	return versionDTO{
		ID: v.ID, Seq: v.Seq, Label: v.Label, OfficialTextURL: v.OfficialTextURL, State: string(v.State),
		CreatedBy: strPtr(v.CreatedBy), CreatedAt: ts(v.CreatedAt),
	}
}

func toContentDTO(c domain.Content) contentDTO {
	return contentDTO{
		VersionID: c.VersionID, Summary: c.Summary, ShortSummary: c.ShortSummary, TopicIDs: nonNil(c.TopicIDs),
		NeedsConfirmation: c.NeedsConfirmation, Authors: nonNil(c.Authors), Revision: c.Revision,
		UpdatedBy: strPtr(c.UpdatedBy), UpdatedAt: ts(c.UpdatedAt),
	}
}

func toTopicDTO(t domain.Topic) topicDTO {
	return topicDTO{ID: t.ID, Name: t.Name, Active: t.Active, CreatedAt: ts(t.CreatedAt)}
}

func actorID(r *http.Request) string {
	a, _ := auth.ActorFrom(r.Context())
	return a.UserID
}

func toEditorialDTO(ed app.TeamEditorial, actor string) editorialDTO {
	out := editorialDTO{
		Versions: make([]versionDTO, 0, len(ed.Versions)), Topics: []topicDTO{},
		CanEdit: ed.CanEdit, CanCreateVersion: ed.CanCreateVersion,
		CanApprove: ed.Review.CanApprove, ApproveBlockers: make([]blockerDTO, 0, len(ed.Review.Blockers)),
		ApprovalBreaksFourEyes: ed.Review.ApprovalBreaksFourEyes, CanSecondReview: ed.Review.CanSecondReview,
		CanMinorFix: ed.CanMinorFix, CanProposeErratum: ed.Review.CanProposeErratum,
		CanEditStructuring: ed.CanEditStructuring, CanRetryStructuring: ed.CanRetryStructuring,
		MinorFixes: make([]minorFixDTO, 0, len(ed.Review.MinorFixes)), Errata: make([]erratumDTO, 0, len(ed.Review.Errata)),
		People: ed.People,
	}
	for _, f := range ed.Review.MinorFixes {
		out.MinorFixes = append(out.MinorFixes, minorFixDTO{
			ID: f.ID, VersionID: f.VersionID, Fields: nonNil(f.Fields), Before: toSnapshotDTO(f.Before), After: toSnapshotDTO(f.After),
			CreatedBy: f.CreatedBy, CreatedAt: ts(f.CreatedAt), CanCheckMinorFix: f.CanCheck,
		})
	}
	if e := ed.Review.PendingErratum; e != nil {
		d := toErratumDTO(*e)
		out.PendingErratum = &d
	}
	for _, e := range ed.Review.Errata {
		out.Errata = append(out.Errata, toErratumDTO(e))
	}
	if out.People == nil {
		out.People = map[string]string{}
	}
	current, pending := domain.FindVersions(ed.Versions)
	for _, v := range ed.Versions {
		out.Versions = append(out.Versions, toVersionDTO(v))
	}
	if pending != nil {
		if c, ok := ed.Contents[pending.ID]; ok {
			d := toContentDTO(c)
			out.PendingContent = &d
		}
	}
	if current != nil {
		if c, ok := ed.Contents[current.ID]; ok {
			d := toContentDTO(c)
			out.CurrentContent = &d
		}
		if st, ok := ed.Structurings[current.ID]; ok {
			d := toStructuringDTO(st)
			out.CurrentStructuring = &d
		}
	}
	if pending != nil {
		if st, ok := ed.Structurings[pending.ID]; ok {
			d := toStructuringDTO(st)
			out.PendingStructuring = &d
		}
	}
	for _, t := range ed.Topics {
		out.Topics = append(out.Topics, toTopicDTO(t))
	}
	sortTopics(out.Topics)
	if c := ed.Claim; c != nil {
		out.Claim = &teamClaimDTO{
			HolderID: c.UserID, HolderName: c.HolderName, ClaimedAt: ts(c.ClaimedAt),
			LastActivityAt: ts(c.LastActivityAt), ExpiresAt: ts(c.ExpiresAt), Mine: c.UserID == actor,
			CanRelease: c.CanRelease,
		}
	}
	if a := ed.Review.Current; a != nil {
		b := a.Badge
		out.ReviewBadge = &b
		out.CurrentApproval = &approvalInfoDTO{
			VersionID: a.VersionID, ApprovedBy: a.ApprovedBy, ApprovedAt: ts(a.ApprovedAt), FourEyes: a.FourEyes,
			SecondReviewBy: strPtr(a.SecondReviewBy), SecondReviewAt: tsPtr(a.SecondReviewAt),
			PendingSecondReview: a.PendingSecondReview,
		}
	}
	for _, b := range ed.Review.Blockers {
		out.ApproveBlockers = append(out.ApproveBlockers, blockerDTO(b))
	}
	return out
}

func sortTopics(ts []topicDTO) {
	sort.Slice(ts, func(i, j int) bool {
		if ts[i].Name != ts[j].Name {
			return ts[i].Name < ts[j].Name
		}
		return ts[i].ID < ts[j].ID
	})
}

// --- Versões e conteúdo ---

type createVersionRequest struct {
	Label           string `json:"label"`
	OfficialTextURL string `json:"official_text_url"`
}

func (h *Handlers) createVersion(w http.ResponseWriter, r *http.Request) {
	var req createVersionRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	res, err := h.uc.CreateVersion.Execute(r.Context(), app.CreateVersionInput{
		PropositionID: chi.URLParam(r, "id"), Label: req.Label, OfficialTextURL: req.OfficialTextURL,
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"version": toVersionDTO(res.Version), "content": toContentDTO(res.Content),
	})
}

type updateContentRequest struct {
	Summary      string   `json:"summary"`
	ShortSummary string   `json:"short_summary"`
	TopicIDs     []string `json:"topic_ids"`
	Revision     *int     `json:"revision"`
}

func (h *Handlers) updateContent(w http.ResponseWriter, r *http.Request) {
	var req updateContentRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Revision == nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Informe a revisão lida (revision).")
		return
	}
	c, err := h.uc.UpdateContent.Execute(r.Context(), app.UpdateContentInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"),
		Summary: req.Summary, ShortSummary: req.ShortSummary, TopicIDs: req.TopicIDs, Revision: *req.Revision,
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toContentDTO(c))
}

type confirmContentRequest struct {
	Revision *int `json:"revision"`
}

func (h *Handlers) confirmContent(w http.ResponseWriter, r *http.Request) {
	var req confirmContentRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Revision == nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Informe a revisão lida (revision).")
		return
	}
	c, err := h.uc.ConfirmContent.Execute(r.Context(), app.ConfirmContentInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"), Revision: *req.Revision,
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toContentDTO(c))
}

// --- Estruturação ---

type reprocessStructuringRequest struct {
	SourceText string `json:"source_text"`
	Revision   *int   `json:"revision"`
}

func (h *Handlers) reprocessStructuring(w http.ResponseWriter, r *http.Request) {
	var req reprocessStructuringRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Revision == nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Informe a revisão lida (revision).")
		return
	}
	v, err := h.uc.ReprocessStructuring.Execute(r.Context(), app.ReprocessStructuringInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"),
		SourceText: req.SourceText, Revision: *req.Revision,
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toStructuringDTO(v))
}

func (h *Handlers) confirmStructuring(w http.ResponseWriter, r *http.Request) {
	var req confirmContentRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Revision == nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Informe a revisão lida (revision).")
		return
	}
	v, err := h.uc.ConfirmStructuring.Execute(r.Context(), app.ConfirmStructuringInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"), Revision: *req.Revision,
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toStructuringDTO(v))
}

func (h *Handlers) retryStructuring(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.RetryStructuring.Execute(r.Context(), app.RetryStructuringInput{
		PropositionID: chi.URLParam(r, "id"), VersionID: chi.URLParam(r, "versionId"),
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toStructuringDTO(v))
}

// --- Temas ---

func (h *Handlers) listTopics(w http.ResponseWriter, r *http.Request) {
	ts, err := h.uc.ListTopics.Execute(r.Context())
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	out := make([]topicDTO, 0, len(ts))
	for _, t := range ts {
		out = append(out, toTopicDTO(t))
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"topics": out})
}

type createTopicRequest struct {
	Name string `json:"name"`
}

func (h *Handlers) createTopic(w http.ResponseWriter, r *http.Request) {
	var req createTopicRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	t, err := h.uc.CreateTopic.Execute(r.Context(), app.CreateTopicInput{Name: req.Name})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toTopicDTO(t))
}

type updateTopicRequest struct {
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
}

func (h *Handlers) updateTopic(w http.ResponseWriter, r *http.Request) {
	var req updateTopicRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	t, err := h.uc.UpdateTopic.Execute(r.Context(), app.UpdateTopicInput{
		TopicID: chi.URLParam(r, "topicId"), Name: req.Name, Active: req.Active,
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTopicDTO(t))
}
