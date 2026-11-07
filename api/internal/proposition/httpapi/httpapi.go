// Package httpapi é o adaptador HTTP do módulo proposition: as rotas da
// equipe sob /api/team (autenticadas, Editor ou acima) e as rotas públicas
// sob /api/public, que só falam com a camada pública (app.ListPublic*,
// app.GetPublic*) e só serializam domain.PublicProposition.
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// UseCases reúne os casos de uso que o HTTP dirige.
type UseCases struct {
	ListPropositions *app.ListPropositions
	GetProposition   *app.GetProposition
	ConfirmLink      *app.ConfirmLink
	RejectLink       *app.RejectLink
	SubmitForReview  *app.SubmitForReview
	ListPublic       *app.ListPublicPropositions
	GetPublic        *app.GetPublicProposition
}

// Handlers são os handlers HTTP do módulo.
type Handlers struct {
	uc     UseCases
	logger *slog.Logger
}

func New(uc UseCases, logger *slog.Logger) *Handlers { return &Handlers{uc: uc, logger: logger} }

// RegisterTeam monta as rotas da equipe em r (o roteador de /api/team, já
// atrás da autenticação).
func (h *Handlers) RegisterTeam(r chi.Router) {
	r.Group(func(r chi.Router) {
		// Recusa cedo; os casos de uso conferem de novo.
		r.Use(auth.RequireRole(auth.RoleEditor))
		r.Get("/propositions", h.list)
		r.Get("/propositions/{id}", h.get)
		r.Post("/propositions/{id}/submit-for-review", h.submitForReview)
		r.Post("/propositions/{id}/links/{linkId}/confirm", h.confirmLink)
		r.Post("/propositions/{id}/links/{linkId}/reject", h.rejectLink)
	})
}

// RegisterPublic monta as rotas públicas em r (o roteador de /api/public).
func (h *Handlers) RegisterPublic(r chi.Router) {
	r.Get("/propositions", h.listPublic)
	r.Get("/propositions/{slug}", h.getPublic)
}

// --- equipe ---

type attachedRefDTO struct {
	ID   *string `json:"id"`
	Name string  `json:"name"`
}

type linkDTO struct {
	ID            string  `json:"id"`
	Chamber       string  `json:"chamber"`
	ExternalID    string  `json:"external_id"`
	State         string  `json:"state"`
	SourceName    string  `json:"source_name"`
	SourceURL     string  `json:"source_url"`
	ConfirmedBy   *string `json:"confirmed_by"`
	ConfirmedAt   *string `json:"confirmed_at"`
	LastSyncedAt  *string `json:"last_synced_at"`
	LastSyncError string  `json:"last_sync_error"`
	CreatedAt     string  `json:"created_at"`
}

type teamPropositionDTO struct {
	ID              string          `json:"id"`
	Slug            string          `json:"slug"`
	Name            string          `json:"name"`
	Kind            string          `json:"kind"`
	Number          int             `json:"number"`
	Year            int             `json:"year"`
	Sphere          string          `json:"sphere"`
	OriginChamber   string          `json:"origin_chamber"`
	Summary         string          `json:"summary"`
	PresentedAt     *string         `json:"presented_at"`
	OfficialTextURL string          `json:"official_text_url"`
	Authors         []string        `json:"authors"`
	Status          string          `json:"status"`
	Superseded      bool            `json:"superseded"`
	AttachedTo      *attachedRefDTO `json:"attached_to"`
	ReviewState     string          `json:"review_state"`
	Links           []linkDTO       `json:"links"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

type timelineDTO struct {
	Chamber     string `json:"chamber"`
	At          string `json:"at"`
	Body        string `json:"body"`
	Description string `json:"description"`
	Situation   string `json:"situation"`
	Dispatch    string `json:"dispatch"`
}

type teamPropositionDetailDTO struct {
	teamPropositionDTO
	Attached []attachedRefDTO `json:"attached"`
	Timeline []timelineDTO    `json:"timeline"`
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func tsPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := ts(*t)
	return &s
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toLinkDTO(l domain.SourceLink) linkDTO {
	return linkDTO{
		ID: l.ID, Chamber: string(l.Chamber), ExternalID: l.ExternalID, State: string(l.State),
		SourceName: l.SourceName, SourceURL: l.SourceURL, ConfirmedBy: strPtr(l.ConfirmedBy),
		ConfirmedAt: tsPtr(l.ConfirmedAt), LastSyncedAt: tsPtr(l.LastSyncedAt), LastSyncError: l.LastSyncError,
		CreatedAt: ts(l.CreatedAt),
	}
}

func toTeamDTO(tp app.TeamProposition) teamPropositionDTO {
	p := tp.Proposition
	out := teamPropositionDTO{
		ID: p.ID, Slug: p.Slug, Name: p.Name(), Kind: p.Kind, Number: p.Number, Year: p.Year,
		Sphere: string(p.Sphere), OriginChamber: string(p.OriginChamber), Summary: p.Summary,
		PresentedAt: tsPtr(p.PresentedAt), OfficialTextURL: p.OfficialTextURL, Authors: nonNil(p.Authors),
		Status: p.Status, Superseded: p.Superseded, ReviewState: string(p.ReviewState),
		Links: make([]linkDTO, 0, len(tp.Links)), CreatedAt: ts(p.CreatedAt), UpdatedAt: ts(p.UpdatedAt),
	}
	if p.AttachedToID != nil || p.AttachedToName != "" {
		out.AttachedTo = &attachedRefDTO{ID: p.AttachedToID, Name: p.AttachedToName}
	}
	for _, l := range tp.Links {
		out.Links = append(out.Links, toLinkDTO(l))
	}
	return out
}

func toTimelineDTO(es []app.TimelineEntry) []timelineDTO {
	out := make([]timelineDTO, 0, len(es))
	for _, e := range es {
		out = append(out, timelineDTO{
			Chamber: e.Chamber, At: ts(e.At), Body: e.Body, Description: e.Description,
			Situation: e.Situation, Dispatch: e.Dispatch,
		})
	}
	return out
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	ps, err := h.uc.ListPropositions.Execute(r.Context(), app.ListPropositionsInput{})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	out := make([]teamPropositionDTO, 0, len(ps))
	for _, p := range ps {
		out = append(out, toTeamDTO(p))
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"propositions": out})
}

func (h *Handlers) get(w http.ResponseWriter, r *http.Request) {
	d, err := h.uc.GetProposition.Execute(r.Context(), app.GetPropositionInput{PropositionID: chi.URLParam(r, "id")})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	out := teamPropositionDetailDTO{
		teamPropositionDTO: toTeamDTO(d.TeamProposition),
		Attached:           make([]attachedRefDTO, 0, len(d.Attached)),
		Timeline:           toTimelineDTO(d.Timeline),
	}
	if d.Principal != nil && out.AttachedTo != nil {
		out.AttachedTo.Name = d.Principal.Name()
	}
	for _, a := range d.Attached {
		id := a.ID
		out.Attached = append(out.Attached, attachedRefDTO{ID: &id, Name: a.Name()})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) submitForReview(w http.ResponseWriter, r *http.Request) {
	p, err := h.uc.SubmitForReview.Execute(r.Context(), app.SubmitForReviewInput{PropositionID: chi.URLParam(r, "id")})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": p.ID, "review_state": string(p.ReviewState)})
}

func (h *Handlers) confirmLink(w http.ResponseWriter, r *http.Request) {
	l, err := h.uc.ConfirmLink.Execute(r.Context(), app.ConfirmLinkInput{
		PropositionID: chi.URLParam(r, "id"), LinkID: chi.URLParam(r, "linkId"),
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toLinkDTO(l))
}

func (h *Handlers) rejectLink(w http.ResponseWriter, r *http.Request) {
	err := h.uc.RejectLink.Execute(r.Context(), app.RejectLinkInput{
		PropositionID: chi.URLParam(r, "id"), LinkID: chi.URLParam(r, "linkId"),
	})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- público ---

// Os DTOs públicos são montados só a partir de domain.PublicProposition:
// campo interno não tem de onde sair.

type publicRefDTO struct {
	Name string  `json:"name"`
	Slug *string `json:"slug"`
}

type publicSourceDTO struct {
	Chamber string `json:"chamber"`
	Name    string `json:"name"`
	URL     string `json:"url"`
}

type publicPropositionDTO struct {
	Slug            string            `json:"slug"`
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Number          int               `json:"number"`
	Year            int               `json:"year"`
	Sphere          string            `json:"sphere"`
	OriginChamber   string            `json:"origin_chamber"`
	Summary         string            `json:"summary"`
	PresentedAt     *string           `json:"presented_at"`
	OfficialTextURL string            `json:"official_text_url"`
	Authors         []string          `json:"authors"`
	Status          string            `json:"status"`
	Superseded      bool              `json:"superseded"`
	AttachedTo      *publicRefDTO     `json:"attached_to"`
	Attached        []publicRefDTO    `json:"attached"`
	Sources         []publicSourceDTO `json:"sources"`
	UnderReview     bool              `json:"under_review"`
	UpdatedAt       string            `json:"updated_at"`
}

type publicPropositionDetailDTO struct {
	publicPropositionDTO
	Timeline []timelineDTO `json:"timeline"`
}

func toPublicRef(r domain.PublicRef) publicRefDTO {
	return publicRefDTO{Name: r.Name, Slug: strPtr(r.Slug)}
}

func toPublicDTO(p domain.PublicProposition) publicPropositionDTO {
	out := publicPropositionDTO{
		Slug: p.Slug, Name: p.Name, Kind: p.Kind, Number: p.Number, Year: p.Year,
		Sphere: string(p.Sphere), OriginChamber: string(p.OriginChamber), Summary: p.Summary,
		PresentedAt: tsPtr(p.PresentedAt), OfficialTextURL: p.OfficialTextURL, Authors: nonNil(p.Authors),
		Status: p.Status, Superseded: p.Superseded, UnderReview: p.UnderReview, UpdatedAt: ts(p.UpdatedAt),
		Attached: make([]publicRefDTO, 0, len(p.Attached)), Sources: make([]publicSourceDTO, 0, len(p.Sources)),
	}
	if p.AttachedTo != nil {
		ref := toPublicRef(*p.AttachedTo)
		out.AttachedTo = &ref
	}
	for _, a := range p.Attached {
		out.Attached = append(out.Attached, toPublicRef(a))
	}
	for _, s := range p.Sources {
		out.Sources = append(out.Sources, publicSourceDTO{Chamber: string(s.Chamber), Name: s.SourceName, URL: s.SourceURL})
	}
	return out
}

// publicCache: curto, para a prévia de link e o SSR aguentarem pico sem
// segurar por muito tempo uma Tramitação nova.
const publicCache = "public, max-age=60"

func (h *Handlers) listPublic(w http.ResponseWriter, r *http.Request) {
	ps, err := h.uc.ListPublic.Execute(r.Context())
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	out := make([]publicPropositionDTO, 0, len(ps))
	for _, p := range ps {
		out = append(out, toPublicDTO(p))
	}
	w.Header().Set("Cache-Control", publicCache)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"propositions": out})
}

func (h *Handlers) getPublic(w http.ResponseWriter, r *http.Request) {
	d, err := h.uc.GetPublic.Execute(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	w.Header().Set("Cache-Control", publicCache)
	httpx.WriteJSON(w, http.StatusOK, publicPropositionDetailDTO{
		publicPropositionDTO: toPublicDTO(d.PublicProposition),
		Timeline:             toTimelineDTO(d.Timeline),
	})
}

// --- erros ---

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
		{domain.ErrPropositionNotFound, http.StatusNotFound, "proposition_not_found", "Proposição não encontrada."},
		{domain.ErrLinkNotFound, http.StatusNotFound, "link_not_found", "Vínculo de fonte não encontrado."},
		{domain.ErrLinkNotSuggested, http.StatusConflict, "link_not_suggested", "Este vínculo não é mais uma sugestão."},
		{domain.ErrLinkTaken, http.StatusConflict, "link_taken", "Este registro já está vinculado a outra Proposição."},
		{domain.ErrChamberAlreadyLinked, http.StatusConflict, "chamber_already_linked", "A Proposição já tem um vínculo confirmado nessa Casa."},
		{domain.ErrInvalidTransition, http.StatusConflict, "invalid_review_transition", "A Proposição não está no estado certo para isso."},
	}
}

func writeErr(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	for _, m := range errMappings() {
		if errors.Is(err, m.err) {
			httpx.WriteError(w, m.status, m.code, m.message)
			return
		}
	}
	logger.ErrorContext(r.Context(), "proposition request failed", "error", err.Error())
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Erro interno. Tente de novo.")
}
