// Package httpapi é o adaptador HTTP do ingest: busca nas fontes oficiais
// e Importação, sob /api/team (autenticado, Editor ou acima).
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
)

// UseCases reúne os casos de uso que o HTTP dirige.
type UseCases struct {
	SearchSources *app.SearchSources
	Import        *app.Import
	ListRadar     *app.ListRadar
	DismissRadar  *app.DismissRadarItem
	RequestRadar  *app.RequestRadarScan
}

// Handlers são os handlers HTTP do ingest.
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
		r.Use(auth.RequireRole(auth.RoleEditor))
		r.Get("/sources/search", h.search)
		r.Post("/propositions/import", h.importProposition)
		r.Get("/radar", h.radar)
		r.Post("/radar/scan", h.requestRadar)
		r.Post("/radar/{chamber}/{externalId}/dismiss", h.dismissRadar)
	})
}

type radarItemDTO struct {
	Chamber     string  `json:"chamber"`
	ExternalID  string  `json:"external_id"`
	Name        string  `json:"name"`
	Summary     string  `json:"summary"`
	SourceURL   string  `json:"source_url"`
	AgendaAt    *string `json:"agenda_at"`
	PresentedAt *string `json:"presented_at"`
}

func tsPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func (h *Handlers) radar(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.ListRadar.Execute(r.Context())
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	items := make([]radarItemDTO, 0, len(v.Items))
	for _, it := range v.Items {
		items = append(items, radarItemDTO{
			Chamber: string(it.Chamber), ExternalID: it.ExternalID,
			Name:    it.Kind + " " + strconv.Itoa(it.Number) + "/" + strconv.Itoa(it.Year),
			Summary: it.Summary, SourceURL: it.SourceURL, AgendaAt: tsPtr(it.AgendaAt), PresentedAt: tsPtr(it.PresentedAt),
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "last_scan_at": tsPtr(v.LastSeen)})
}

func (h *Handlers) requestRadar(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.RequestRadar.Execute(r.Context()); err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handlers) dismissRadar(w http.ResponseWriter, r *http.Request) {
	err := h.uc.DismissRadar.Execute(r.Context(), app.ChamberCode(chi.URLParam(r, "chamber")), chi.URLParam(r, "externalId"))
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type hitDTO struct {
	ExternalID          string   `json:"external_id"`
	Name                string   `json:"name"`
	Kind                string   `json:"kind"`
	Number              int      `json:"number"`
	Year                int      `json:"year"`
	OfficialSummary     string   `json:"official_summary"`
	PresentedAt         *string  `json:"presented_at"`
	Authors             []string `json:"authors"`
	Status              string   `json:"status"`
	SourceURL           string   `json:"source_url"`
	LinkedPropositionID *string  `json:"linked_proposition_id"`
	LinkState           *string  `json:"link_state"`
}

type chamberResultDTO struct {
	Chamber     string   `json:"chamber"`
	Unavailable bool     `json:"unavailable"`
	Hits        []hitDTO `json:"hits"`
}

func (h *Handlers) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	number, errN := strconv.Atoi(q.Get("number"))
	year, errY := strconv.Atoi(q.Get("year"))
	if errN != nil || errY != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_query", "Informe tipo, número e ano (ex.: PEC 221/2019).")
		return
	}
	res, err := h.uc.SearchSources.Execute(r.Context(), app.SearchSourcesInput{Kind: q.Get("kind"), Number: number, Year: year})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	out := make([]chamberResultDTO, 0, len(res))
	for _, c := range res {
		dto := chamberResultDTO{Chamber: string(c.Chamber), Unavailable: c.Unavailable, Hits: []hitDTO{}}
		for _, hit := range c.Hits {
			rec := hit.Record
			d := hitDTO{
				ExternalID: rec.ExternalID, Name: officialName(rec), Kind: rec.Kind, Number: rec.Number, Year: rec.Year,
				OfficialSummary: rec.OfficialSummary, Authors: nonNil(rec.Authors), Status: rec.Status, SourceURL: rec.SourceURL,
			}
			if !rec.PresentedAt.IsZero() {
				s := rec.PresentedAt.UTC().Format(time.RFC3339)
				d.PresentedAt = &s
			}
			if hit.LinkedPropositionID != "" {
				id, state := hit.LinkedPropositionID, "suggested"
				if hit.LinkConfirmed {
					state = "confirmed"
				}
				d.LinkedPropositionID, d.LinkState = &id, &state
			}
			dto.Hits = append(dto.Hits, d)
		}
		out = append(out, dto)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": out})
}

type importResultDTO struct {
	PropositionID  string `json:"proposition_id"`
	Created        bool   `json:"created"`
	SuggestedLinks int    `json:"suggested_links"`
	// SuggestionsUnavailable: Casas que não responderam à procura de
	// candidatos; a Proposição foi importada sem sugestões delas.
	SuggestionsUnavailable []string `json:"suggestions_unavailable"`
}

func (h *Handlers) importProposition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Chamber    string `json:"chamber"`
		ExternalID string `json:"external_id"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	res, err := h.uc.Import.Execute(r.Context(), app.ImportInput{Chamber: req.Chamber, ExternalID: req.ExternalID})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	out := importResultDTO{
		PropositionID: res.PropositionID, Created: res.Created, SuggestedLinks: res.Suggested,
		SuggestionsUnavailable: make([]string, 0, len(res.SuggestionsUnavailable)),
	}
	for _, c := range res.SuggestionsUnavailable {
		out.SuggestionsUnavailable = append(out.SuggestionsUnavailable, string(c))
	}
	httpx.WriteJSON(w, status, out)
}

func officialName(r app.SourceRecord) string {
	return r.Kind + " " + strconv.Itoa(r.Number) + "/" + strconv.Itoa(r.Year)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type errMapping struct {
	err     error
	status  int
	code    string
	message string
}

// errMappings traduz erros para o formato padrão. A mensagem vai para a
// tela; detalhes da fonte (URL, status HTTP) ficam só no log.
func errMappings() []errMapping {
	return []errMapping{
		{app.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated", "Faça login para continuar."},
		{app.ErrForbidden, http.StatusForbidden, "forbidden", "Você não tem permissão para isso."},
		{app.ErrInvalidQuery, http.StatusBadRequest, "invalid_query", "Informe tipo, número e ano (ex.: PEC 221/2019)."},
		{app.ErrUnknownChamber, http.StatusBadRequest, "unknown_chamber", "Casa legislativa sem fonte configurada: use camara ou senado."},
		{app.ErrInvalidID, http.StatusBadRequest, "invalid_external_id", "Identificador do registro na Casa inválido."},
		{app.ErrAlreadySuggested, http.StatusConflict, "already_suggested", "Este registro já foi sugerido como vínculo de uma Proposição. Confirme o vínculo nela."},
		{app.ErrPropositionExists, http.StatusConflict, "proposition_exists", "Já existe uma Proposição com esse nome, ligada a outro registro."},
		{app.ErrLinkTaken, http.StatusConflict, "link_taken", "Este registro já está vinculado a outra Proposição."},
		{app.ErrNotFound, http.StatusNotFound, "source_record_not_found", "A Casa não tem esse registro."},
		{app.ErrRadarItemNotFound, http.StatusNotFound, "radar_item_not_found", "Item do Radar não encontrado."},
		{app.ErrUnavailable, http.StatusServiceUnavailable, "source_unavailable", "A fonte oficial não respondeu. Nada foi gravado; tente de novo em alguns minutos."},
	}
}

func writeErr(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	for _, m := range errMappings() {
		if errors.Is(err, m.err) {
			if m.status >= 500 {
				logger.WarnContext(r.Context(), "official source failed", "error", err.Error())
			}
			httpx.WriteError(w, m.status, m.code, m.message)
			return
		}
	}
	logger.ErrorContext(r.Context(), "ingest request failed", "error", err.Error())
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Erro interno. Tente de novo.")
}
