package camara

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRadarAgendaAndPresented(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch r.URL.Path {
		case "/api/v2/eventos":
			if q.Get("idOrgao") != "180" || q.Get("dataInicio") != "2026-11-11" || q.Get("dataFim") != "2026-11-18" {
				t.Errorf("eventos = %v", q)
			}
			_, _ = w.Write([]byte(`{"dados":[
			 {"id":1,"dataHoraInicio":"2026-11-11T13:55","situacao":"Convocada","orgaos":[{"sigla":"PLEN"}]},
			 {"id":2,"dataHoraInicio":"2026-11-11T10:00","situacao":"Cancelada","orgaos":[{"sigla":"PLEN"}]},
			 {"id":3,"dataHoraInicio":"2026-11-12T10:00","situacao":"Convocada","orgaos":[{"sigla":"CCJC"}]}],"links":[]}`))
		case "/api/v2/eventos/1/pauta":
			_, _ = w.Write([]byte(`{"dados":[
			 {"proposicao_":{"id":2233802,"siglaTipo":"PEC","numero":221,"ano":2019,"ementa":"Altera"}},
			 {"proposicao_":{"id":555,"siglaTipo":"PRLP","numero":1,"ano":2026,"ementa":"parecer"},
			  "proposicaoRelacionada_":{"id":2400000,"siglaTipo":"PL","numero":10,"ano":2024,"ementa":"Dispõe"}},
			 {"proposicao_":{"id":556,"siglaTipo":"REQ","numero":3,"ano":2026,"ementa":"Requer"}}]}`))
		case "/api/v2/eventos/2/pauta", "/api/v2/eventos/3/pauta":
			t.Errorf("leu pauta de %s", r.URL.Path)
		case "/api/v2/proposicoes":
			if q.Get("siglaTipo") != "PL,PEC,PLP" || q.Get("dataApresentacaoInicio") != "2026-11-08" {
				t.Errorf("apresentadas = %v", q)
			}
			if q.Get("pagina") == "" {
				_, _ = w.Write([]byte(`{"dados":[{"id":2600001,"siglaTipo":"PL","numero":6000,"ano":2026,"ementa":"Institui","dataApresentacao":"2026-11-10T15:12"}],
				 "links":[{"rel":"next","href":"` + srv.URL + `/api/v2/proposicoes?siglaTipo=PL,PEC,PLP&dataApresentacaoInicio=2026-11-08&pagina=2"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"dados":[{"id":2600002,"siglaTipo":"PLP","numero":7,"ano":2026,"ementa":"Altera","dataApresentacao":"2026-11-09T10:00"}],"links":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	s, err := New(srv.Client(), srv.URL+"/api/v2", Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	day := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	agenda, err := s.Agenda(ctx, day, day.AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	if len(agenda) != 2 || agenda[0].ExternalID != "2233802" || agenda[1].ExternalID != "2400000" || agenda[1].Kind != "PL" ||
		agenda[0].AgendaAt.UTC().Format(time.RFC3339) != "2026-11-11T16:55:00Z" {
		t.Fatalf("pauta = %+v", agenda)
	}
	presented, err := s.Presented(ctx, day.AddDate(0, 0, -3), day)
	if err != nil {
		t.Fatal(err)
	}
	if len(presented) != 2 || presented[1].Kind != "PLP" || presented[0].PresentedAt == nil {
		t.Fatalf("apresentadas = %+v", presented)
	}
}
