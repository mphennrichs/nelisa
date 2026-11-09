// Package httpapi é o adaptador HTTP do módulo contribution: envio e
// consulta de Contribuições pelo Leitor, sem login, sob /api/public; e o
// cadastro de Fonte oficial, sob /api/team.
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
)

// Limites por IP. O de envio é provisório (em memória, por instância) até
// o rate limit no Postgres do passo 3 (#27).
const (
	submitPerHour = 5
	lookupPerHour = 30
)

// UseCases reúne os casos de uso que o HTTP dirige.
type UseCases struct {
	Submit               *app.Submit
	Lookup               *app.Lookup
	ListOfficialSources  *app.ListOfficialSources
	CreateOfficialSource *app.CreateOfficialSource
	DeleteOfficialSource *app.DeleteOfficialSource

	GetContribution     *app.GetContribution
	AcceptContribution  *app.AcceptContribution
	RejectContribution  *app.RejectContribution
	CorrectSphere       *app.CorrectSphere
	LinkProposition     *app.LinkProposition
	DeleteProgressEntry *app.DeleteProgressEntry
}

// Options vêm da configuração.
type Options struct {
	// Enabled liga o envio (CONTRIBUTIONS_ENABLED). Desligado, o POST de
	// Contribuição nem existe; a consulta pelo código continua no ar.
	Enabled bool
	// LimitKey é a chave do rate limit por IP (clientip.Resolver.LimitKey).
	LimitKey func(*http.Request) (string, error)
}

// Handlers são os handlers HTTP do módulo.
type Handlers struct {
	uc     UseCases
	opts   Options
	logger *slog.Logger
}

func New(uc UseCases, opts Options, logger *slog.Logger) *Handlers {
	return &Handlers{uc: uc, opts: opts, logger: logger}
}

// RegisterPublic monta as rotas do Leitor em r (o roteador de
// /api/public).
func (h *Handlers) RegisterPublic(r chi.Router) {
	r.Get("/contributions/availability", h.availability)
	r.With(httprate.LimitBy(lookupPerHour, time.Hour, h.opts.LimitKey, httprate.WithLimitHandler(tooManyRequests))).
		Post("/contributions/lookup", h.lookup)
	if h.opts.Enabled {
		r.With(httprate.LimitBy(submitPerHour, time.Hour, h.opts.LimitKey, httprate.WithLimitHandler(tooManyRequests))).
			Post("/contributions", h.submit)
	}
}

// RegisterTeam monta as rotas da equipe em r (o roteador de /api/team, já
// atrás da autenticação).
func (h *Handlers) RegisterTeam(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireRole(auth.RoleEditor))
		r.Get("/official-sources", h.listSources)
		r.Post("/official-sources", h.createSource)
		r.With(auth.RequireRole(auth.RoleAdmin)).Delete("/official-sources/{id}", h.deleteSource)

		// Moderação (#25): as pendentes aparecem na Fila (módulo queue).
		r.Get("/contributions/{id}", h.getContribution)
		r.Post("/contributions/{id}/accept", h.acceptContribution)
		r.Post("/contributions/{id}/reject", h.rejectContribution)
		r.Put("/contributions/{id}/sphere", h.correctSphere)
		r.Put("/contributions/{id}/proposition", h.linkProposition)
		r.Delete("/contributions/{id}/progress-entry", h.deleteProgressEntry)
	})
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func noStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

type availabilityDTO struct {
	Open bool `json:"open"`
	// Reason: "disabled" quando o envio está desligado; null se aberto.
	Reason *string `json:"reason"`
}

func (h *Handlers) availability(w http.ResponseWriter, _ *http.Request) {
	out := availabilityDTO{Open: h.opts.Enabled}
	if !h.opts.Enabled {
		r := "disabled"
		out.Reason = &r
	}
	noStore(w)
	httpx.WriteJSON(w, http.StatusOK, out)
}

type submitRequest struct {
	Kind            string `json:"kind"`
	Link            string `json:"link"`
	Sphere          string `json:"sphere"`
	PropositionSlug string `json:"proposition_slug"`
	EventType       string `json:"event_type"`
	EventDate       string `json:"event_date"`
	Domain          string `json:"domain"`
	SourceName      string `json:"source_name"`
	Note            string `json:"note"`
}

type submitResponse struct {
	TrackingCode string `json:"tracking_code"`
}

func (h *Handlers) submit(w http.ResponseWriter, r *http.Request) {
	var req submitRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	code, err := h.uc.Submit.Execute(r.Context(), app.SubmitInput{
		PropositionSlug: req.PropositionSlug,
		Submission: domain.Submission{
			Kind: domain.Kind(req.Kind), Link: req.Link, Sphere: domain.Sphere(req.Sphere),
			EventType: domain.EventType(req.EventType), EventDate: req.EventDate,
			Domain: req.Domain, SourceName: req.SourceName, Note: req.Note,
		},
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	noStore(w)
	httpx.WriteJSON(w, http.StatusCreated, submitResponse{TrackingCode: code.Display()})
}

type lookupRequest struct {
	Code string `json:"code"`
}

type lookupResponse struct {
	Kind            string  `json:"kind"`
	State           string  `json:"state"`
	SubmittedAt     string  `json:"submitted_at"`
	DecidedAt       *string `json:"decided_at"`
	PublicReason    *string `json:"public_reason"`
	PropositionSlug *string `json:"proposition_slug"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h *Handlers) lookup(w http.ResponseWriter, r *http.Request) {
	var req lookupRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	res, err := h.uc.Lookup.Execute(r.Context(), req.Code)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	out := lookupResponse{
		Kind: string(res.Kind), State: string(res.State), SubmittedAt: ts(res.SubmittedAt),
		PublicReason: optional(res.PublicReason), PropositionSlug: optional(res.PropositionSlug),
	}
	if res.DecidedAt != nil {
		out.DecidedAt = optional(ts(*res.DecidedAt))
	}
	noStore(w)
	httpx.WriteJSON(w, http.StatusOK, out)
}

type sourceDTO struct {
	ID        string `json:"id"`
	Domain    string `json:"domain"`
	Name      string `json:"name"`
	Sphere    string `json:"sphere"`
	CreatedAt string `json:"created_at"`
}

func toSourceDTO(s domain.OfficialSource) sourceDTO {
	return sourceDTO{ID: s.ID, Domain: s.Domain, Name: s.Name, Sphere: string(s.Sphere), CreatedAt: ts(s.CreatedAt)}
}

func (h *Handlers) listSources(w http.ResponseWriter, r *http.Request) {
	ss, err := h.uc.ListOfficialSources.Execute(r.Context())
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	out := make([]sourceDTO, 0, len(ss))
	for _, s := range ss {
		out = append(out, toSourceDTO(s))
	}
	noStore(w)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items": out, "default_suffixes": domain.DefaultOfficialSuffixes,
	})
}

type createSourceRequest struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Sphere string `json:"sphere"`
}

func (h *Handlers) createSource(w http.ResponseWriter, r *http.Request) {
	var req createSourceRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	s, err := h.uc.CreateOfficialSource.Execute(r.Context(), app.CreateOfficialSourceInput{
		Domain: req.Domain, Name: req.Name, Sphere: domain.Sphere(req.Sphere),
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toSourceDTO(s))
}

func (h *Handlers) deleteSource(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.DeleteOfficialSource.Execute(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Moderação ---

type propRefDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func toPropRef(p *app.TeamProposition) *propRefDTO {
	if p == nil {
		return nil
	}
	return &propRefDTO{ID: p.ID, Name: p.Name, Slug: p.Slug}
}

// contributionDTO é a Contribuição para a equipe: o texto do Leitor vai
// como veio, e a tela o mostra como texto puro.
type contributionDTO struct {
	ID                 string      `json:"id"`
	Kind               string      `json:"kind"`
	State              string      `json:"state"`
	Sphere             string      `json:"sphere"`
	SubmittedAt        string      `json:"submitted_at"`
	Link               string      `json:"link"`
	LinkHost           string      `json:"link_host"`
	EventType          *string     `json:"event_type"`
	EventDate          *string     `json:"event_date"`
	Domain             *string     `json:"domain"`
	SourceName         *string     `json:"source_name"`
	Note               string      `json:"note"`
	Proposition        *propRefDTO `json:"proposition"`
	CreatedProposition *propRefDTO `json:"created_proposition"`
	DecidedAt          *string     `json:"decided_at"`
	PublicReason       *string     `json:"public_reason"`
	InternalNote       string      `json:"internal_note"`
}

func toContributionDTO(v app.ContributionView) contributionDTO {
	host := ""
	if l, err := domain.ParseLink(v.Payload.Link); err == nil {
		host = l.Host
	}
	d := contributionDTO{
		ID: v.ID, Kind: string(v.Kind), State: string(v.State), Sphere: string(v.Sphere), SubmittedAt: ts(v.SubmittedAt),
		Link: v.Payload.Link, LinkHost: host, EventType: optional(string(v.Payload.EventType)),
		EventDate: optional(v.Payload.EventDate), Domain: optional(v.Payload.Domain),
		SourceName: optional(v.Payload.SourceName), Note: v.Payload.Note,
		Proposition: toPropRef(v.Proposition), CreatedProposition: toPropRef(v.CreatedProposition),
		PublicReason: optional(v.PublicReason), InternalNote: v.InternalNote,
	}
	if v.DecidedAt != nil {
		d.DecidedAt = optional(ts(*v.DecidedAt))
	}
	return d
}

func (h *Handlers) writeContribution(w http.ResponseWriter, v app.ContributionView) {
	noStore(w)
	httpx.WriteJSON(w, http.StatusOK, toContributionDTO(v))
}

func (h *Handlers) getContribution(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.GetContribution.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeTeamErr(w, r, err)
		return
	}
	h.writeContribution(w, v)
}

type eventRequest struct {
	Chamber     string `json:"chamber"`
	Date        string `json:"date"`
	Description string `json:"description"`
	SourceURL   string `json:"source_url"`
}

type acceptRequest struct {
	InternalNote string        `json:"internal_note"`
	Event        *eventRequest `json:"event"`
}

func (h *Handlers) acceptContribution(w http.ResponseWriter, r *http.Request) {
	var req acceptRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	in := app.AcceptInput{ID: chi.URLParam(r, "id"), InternalNote: req.InternalNote}
	if req.Event != nil {
		in.Event = &app.ProgressEventInput{
			Chamber: req.Event.Chamber, Date: req.Event.Date, Description: req.Event.Description, SourceURL: req.Event.SourceURL,
		}
	}
	v, err := h.uc.AcceptContribution.Execute(r.Context(), in)
	if err != nil {
		h.writeTeamErr(w, r, err)
		return
	}
	h.writeContribution(w, v)
}

type rejectRequest struct {
	Reason       string `json:"reason"`
	InternalNote string `json:"internal_note"`
}

func (h *Handlers) rejectContribution(w http.ResponseWriter, r *http.Request) {
	var req rejectRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, err := h.uc.RejectContribution.Execute(r.Context(), app.RejectInput{
		ID: chi.URLParam(r, "id"), Reason: domain.RejectReason(req.Reason), InternalNote: req.InternalNote,
	})
	if err != nil {
		h.writeTeamErr(w, r, err)
		return
	}
	h.writeContribution(w, v)
}

type sphereRequest struct {
	Sphere string `json:"sphere"`
}

func (h *Handlers) correctSphere(w http.ResponseWriter, r *http.Request) {
	var req sphereRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, err := h.uc.CorrectSphere.Execute(r.Context(), app.CorrectSphereInput{ID: chi.URLParam(r, "id"), Sphere: domain.Sphere(req.Sphere)})
	if err != nil {
		h.writeTeamErr(w, r, err)
		return
	}
	h.writeContribution(w, v)
}

type linkRequest struct {
	PropositionID string `json:"proposition_id"`
}

func (h *Handlers) linkProposition(w http.ResponseWriter, r *http.Request) {
	var req linkRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, err := h.uc.LinkProposition.Execute(r.Context(), app.LinkPropositionInput{ID: chi.URLParam(r, "id"), PropositionID: req.PropositionID})
	if err != nil {
		h.writeTeamErr(w, r, err)
		return
	}
	h.writeContribution(w, v)
}

func (h *Handlers) deleteProgressEntry(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.DeleteProgressEntry.Execute(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.writeTeamErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeTeamErr: as mensagens da moderação. Contribuição inexistente aqui
// não é "confira o código" (isso é da consulta do Leitor).
func (h *Handlers) writeTeamErr(w http.ResponseWriter, r *http.Request, err error) {
	bad, conflict := http.StatusBadRequest, http.StatusConflict
	for _, m := range []errMapping{
		{domain.ErrContributionNotFound, http.StatusNotFound, "contribution_not_found", "Contribuição não encontrada."},
		{app.ErrPropositionGone, http.StatusNotFound, "proposition_not_found", "Proposição não encontrada."},
		{domain.ErrNotPending, conflict, "contribution_decided", "Esta Contribuição já foi decidida. Recarregue a página."},
		{domain.ErrNotLinkable, conflict, "not_linkable", "Só uma Indicação aceita, ainda sem Proposição, aponta a Proposição criada."},
		{domain.ErrNoProgressEntry, http.StatusNotFound, "progress_entry_not_found", "Esta Contribuição não tem evento na Tramitação (ou ele já foi apagado)."},
		{app.ErrProgressEntryExists, conflict, "progress_entry_exists", "O evento desta Contribuição já está na Tramitação."},
		{domain.ErrSphereFixed, bad, "sphere_fixed", "A Esfera de uma Atualização de tramitação vem da Proposição."},
		{domain.ErrInvalidReason, bad, "invalid_reason", "Escolha o motivo da rejeição."},
		{domain.ErrInternalNoteLen, bad, "internal_note_too_long", "A nota interna passa de 1.000 caracteres."},
		{app.ErrEventRequired, bad, "event_required", "Revise o evento antes de aceitar."},
		{domain.ErrInvalidChamber, bad, "invalid_chamber", "Escolha a Casa onde o evento aconteceu."},
		{domain.ErrInvalidEventText, bad, "invalid_event_text", "Descreva o evento (até 1.000 caracteres)."},
		{domain.ErrInvalidLink, bad, "invalid_link", "O link da fonte precisa ser completo, começando com https://"},
		{domain.ErrOfficialSourceExists, conflict, "official_source_exists", "Este domínio já é Fonte oficial. Rejeite o pedido como \"já cadastrado\"."},
	} {
		if errors.Is(err, m.err) {
			httpx.WriteError(w, m.status, m.code, m.message)
			return
		}
	}
	h.writeErr(w, r, err)
}

type errMapping struct {
	err     error
	status  int
	code    string
	message string
}

func errMappings() []errMapping {
	bad := http.StatusBadRequest
	return []errMapping{
		{app.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated", "Faça login para continuar."},
		{app.ErrForbidden, http.StatusForbidden, "forbidden", "Você não tem permissão para isso."},
		{domain.ErrContributionNotFound, http.StatusNotFound, "contribution_not_found", "Nenhuma Contribuição com este código. Confira o código e tente de novo."},
		{domain.ErrOfficialSourceNotFound, http.StatusNotFound, "official_source_not_found", "Fonte oficial não encontrada."},
		{domain.ErrInvalidKind, bad, "invalid_kind", "Escolha o tipo de Contribuição."},
		{domain.ErrInvalidLink, bad, "invalid_link", "Informe o link completo, começando com https://"},
		{domain.ErrInvalidDomain, bad, "invalid_domain", "Informe só o domínio, por exemplo camara.exemplo.sp.gov.br."},
		{domain.ErrNotOfficialSource, bad, "not_official_source", "O link precisa ser de uma Fonte oficial: domínio .leg.br, .gov.br ou cadastrado pela equipe. Se a Casa usa outro domínio, mande um Pedido de fonte oficial."},
		{domain.ErrOfficialSourceExists, http.StatusConflict, "official_source_exists", "Este domínio já é Fonte oficial."},
		{domain.ErrInvalidSourceName, bad, "invalid_source_name", "Informe o nome da Casa ou órgão (até 200 caracteres)."},
		{domain.ErrInvalidSphere, bad, "invalid_sphere", "Escolha a Esfera: federal, estadual ou municipal."},
		{domain.ErrInvalidEventType, bad, "invalid_event_type", "Escolha o tipo de evento."},
		{domain.ErrInvalidEventDate, bad, "invalid_event_date", "Informe a data do evento (até um ano à frente)."},
		{domain.ErrNoteTooLong, bad, "note_too_long", "A observação passa de 1.000 caracteres."},
		{domain.ErrPropositionNotFound, http.StatusNotFound, "proposition_not_found", "Proposição não encontrada."},
	}
}

func (h *Handlers) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	for _, m := range errMappings() {
		if errors.Is(err, m.err) {
			httpx.WriteError(w, m.status, m.code, m.message)
			return
		}
	}
	h.logger.ErrorContext(r.Context(), "contribution request failed", "error", err.Error())
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Erro interno. Tente de novo.")
}

func tooManyRequests(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteError(w, http.StatusTooManyRequests, "too_many_requests", "Muitas tentativas deste endereço. Tente de novo mais tarde.")
}
