package senado

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// Respostas cortadas das reais (2026-11-11): o Senado manda objeto quando
// a lista tem um item só.
const agendaDia = `{"AgendaPlenario":{"Sessoes":{"Sessao":[
 {"Data":"2026-11-11","Hora":"14:00","SituacaoSessao":"Agendada","Materias":{"Materia":[
   {"CodigoMateria":"173581","SiglaMateria":"PLP","NumeroMateria":"00109","AnoMateria":"2025","Ementa":"Altera a LC 101"},
   {"CodigoMateria":"176270","SiglaMateria":"RQS","NumeroMateria":"00753","AnoMateria":"2026","Ementa":"Requer"}]}},
 {"Data":"2026-11-11","Hora":"10:00","SituacaoSessao":"Cancelada","Materias":{"Materia":
   {"CodigoMateria":"999","SiglaMateria":"PL","NumeroMateria":"1","AnoMateria":"2026","Ementa":"x"}}},
 {"Data":"2026-11-11","Hora":"18:00","SituacaoSessao":"Agendada","Materias":{"Materia":
   {"CodigoMateria":"162645","SiglaMateria":"PL","NumeroMateria":"05608","AnoMateria":"2023","Ementa":"Dispõe sobre"}}}]}}}`

func TestRadarAgendaAndPresented(t *testing.T) {
	calls := map[string]int{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p, q := r.URL.Path, r.URL.Query()
		calls[p]++
		switch {
		case p == "/dadosabertos/plenario/agenda/dia/20261111.json":
			_, _ = w.Write([]byte(agendaDia))
		case p == "/dadosabertos/plenario/agenda/dia/20261112.json":
			_, _ = w.Write([]byte(`{"AgendaPlenario":{"Metadados":{}}}`))
		case p == "/dadosabertos/processo" && q.Get("codigoMateria") == "173581":
			_, _ = w.Write([]byte(`[{"id":9031010,"codigoMateria":173581,"identificacao":"PLP 109/2025"}]`))
		case p == "/dadosabertos/processo" && q.Get("codigoMateria") == "162645":
			_, _ = w.Write([]byte(`[]`))
		case p == "/dadosabertos/processo" && q.Get("sigla") == "PEC":
			if q.Get("dataInicioApresentacao") != "2026-11-08" || q.Get("dataFimApresentacao") != "2026-11-11" {
				t.Errorf("janela = %v", q)
			}
			_, _ = w.Write([]byte(`[{"id":9100001,"codigoMateria":176300,"identificacao":"PEC 21/2026","ementa":"Altera a Constituição","dataApresentacao":"2026-11-04"},
			  {"id":9100002,"codigoMateria":176301,"identificacao":"PL 5/2026","ementa":"outro tipo"}]`))
		case p == "/dadosabertos/processo":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	s := newSource(t, h, Config{})
	ctx := context.Background()
	day := time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)
	agenda, err := s.Agenda(ctx, day, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(agenda) != 1 || agenda[0].ExternalID != "9031010" || agenda[0].Kind != "PLP" || agenda[0].Number != 109 ||
		agenda[0].AgendaAt.UTC().Format(time.RFC3339) != "2026-11-11T17:00:00Z" || agenda[0].SourceURL == "" {
		t.Fatalf("pauta = %+v", agenda)
	}
	presented, err := s.Presented(ctx, time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC), day)
	if err != nil {
		t.Fatal(err)
	}
	if len(presented) != 1 || presented[0].ExternalID != "9100001" || presented[0].Kind != "PEC" || presented[0].PresentedAt == nil {
		t.Fatalf("apresentadas = %+v", presented)
	}
	if calls["/dadosabertos/processo"] != 5 {
		t.Errorf("chamadas a /processo = %d (2 da pauta + 3 Tipos)", calls["/dadosabertos/processo"])
	}
}
