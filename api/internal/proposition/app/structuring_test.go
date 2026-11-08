package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

const camaraPDF = "https://www.camara.leg.br/proposicoesWeb/prop_mostrarintegra?codteor=1845483"

// fakeText faz o papel da busca e leitura do PDF oficial.
type fakeText struct {
	pages []string
	err   error
	calls int
}

func (f *fakeText) Fetch(context.Context, string) ([]string, error) {
	f.calls++
	return f.pages, f.err
}

func (e *env) run(text app.OfficialText) *app.RunStructuring {
	return app.NewRunStructuring(e.w.Versions(), e.w.Structurings(), text, e.w.Tx(), e.clk)
}

// importWithURL importa uma Proposição cuja fonte deu este link do texto.
func (e *env) importWithURL(t *testing.T, url string) domain.Version {
	t.Helper()
	var res app.CreateImportedResult
	err := e.w.Tx().WithinTx(context.Background(), func(ctx context.Context) error {
		var err error
		res, err = e.catalog().CreateImported(ctx, app.CreateImportedInput{
			Chamber: domain.ChamberCamara, ExternalID: "2192459", SourceName: "PEC 221/2019",
			SourceURL: "https://www.camara.leg.br/proposicoesWeb/fichadetramitacao?idProposicao=2192459",
			Kind:      "PEC", Number: 221, Year: 2019, OriginChamber: domain.ChamberCamara,
			Data:     domain.OfficialData{OfficialSummary: "Altera a jornada", OfficialTextURL: url},
			EditorID: "editor-1",
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return e.pendingOf(t, res.Proposition.ID)
}

const pecText = `PROPOSTA DE EMENDA À CONSTITUIÇÃO Nº , DE 2019
Altera o art. 7º da Constituição Federal.
As Mesas da Câmara dos Deputados e do Senado Federal promulgam a seguinte Emenda:
Art. 1º O art. 7º passa a vigorar com alterações.
Art. 2º Esta Emenda entra em vigor na data de sua publicação.
JUSTIFICAÇÃO
A jornada é longa.`

func TestImportStartsStructuringFromOfficialSource(t *testing.T) {
	e := newEnv()
	v := e.importWithURL(t, camaraPDF)
	s, ok := e.w.StructuringOf(v.ID)
	if !ok || s.State != domain.StructuringFetching || s.Failure != domain.FailureNone {
		t.Fatalf("estruturação = %+v", s)
	}
	if q := e.w.StructuringQueue(); !slices.Equal(q, []string{v.ID}) {
		t.Fatalf("fila = %v", q)
	}
}

func TestStructuringIsManualWithoutOfficialSource(t *testing.T) {
	for _, tt := range []struct {
		name, url string
		want      domain.StructuringFailure
	}{
		{"sem link", "", domain.FailureNoURL},
		{"site municipal", "https://camara.goiania.go.gov.br/pl-1.pdf", domain.FailureHostNotAllowed},
		{"http", "http://www.camara.leg.br/x.pdf", domain.FailureHostNotAllowed},
		{"parecido", "https://www.camara.leg.br.evil.example/x.pdf", domain.FailureHostNotAllowed},
		{"com porta", "https://www.camara.leg.br:8443/x.pdf", domain.FailureHostNotAllowed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv()
			v := e.importWithURL(t, tt.url)
			s, _ := e.w.StructuringOf(v.ID)
			if s.State != domain.StructuringManual || s.Failure != tt.want {
				t.Fatalf("estruturação = %+v, quer manual/%s", s, tt.want)
			}
			if q := e.w.StructuringQueue(); len(q) != 0 {
				t.Fatalf("nada a buscar, mas enfileirou %v", q)
			}
		})
	}
}

func TestNewVersionStartsStructuring(t *testing.T) {
	e := newEnv()
	pid, _ := e.readyWithContent(t)
	res, err := e.createVersion().Execute(ana, app.CreateVersionInput{
		PropositionID: pid, Label: "Substitutivo", OfficialTextURL: "https://www.camara.leg.br/sub.pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := e.w.StructuringOf(res.Version.ID)
	if s.State != domain.StructuringFetching || !slices.Contains(e.w.StructuringQueue(), res.Version.ID) {
		t.Fatalf("estruturação = %+v, fila %v", s, e.w.StructuringQueue())
	}
}

func TestImportRollsBackWhenStructuringCannotBeQueued(t *testing.T) {
	e := newEnv()
	e.w.ScheduleErr = errors.New("river fora")
	err := e.w.Tx().WithinTx(context.Background(), func(ctx context.Context) error {
		_, err := e.catalog().CreateImported(ctx, app.CreateImportedInput{
			Chamber: domain.ChamberCamara, ExternalID: "1", Kind: "PEC", Number: 1, Year: 2020,
			OriginChamber: domain.ChamberCamara, Data: domain.OfficialData{OfficialTextURL: camaraPDF}, EditorID: "editor-1",
		})
		return err
	})
	if _, ok := e.w.StructuringOf("x"); err == nil || ok || len(e.w.AllLinks()) != 0 {
		t.Fatalf("err = %v, vínculos = %d", err, len(e.w.AllLinks()))
	}
}

func TestRunStructuringParsesTheOfficialText(t *testing.T) {
	e := newEnv()
	v := e.importWithURL(t, camaraPDF)
	text := &fakeText{pages: []string{pecText}}
	res, err := e.run(text).Execute(context.Background(), v.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := e.w.StructuringOf(v.ID)
	if s.State != domain.StructuringReadyToCheck || s.Revision != 1 || s.UpdatedBy != "" || !strings.Contains(s.SourceText, "Art. 2º") {
		t.Fatalf("estruturação = %+v", s)
	}
	var addrs []string
	for _, p := range e.w.ProvisionsOf(v.ID) {
		addrs = append(addrs, p.Address)
		if p.VersionID != v.ID || p.TextHash == "" {
			t.Errorf("dispositivo %+v", p)
		}
	}
	if strings.Join(addrs, " ") != "ementa preambulo art-1 art-2 justificacao" || res.Provisions != 5 {
		t.Fatalf("dispositivos = %v (%+v)", addrs, res)
	}

	// Rodar de novo (job repetido) não busca outra vez.
	if _, err := e.run(text).Execute(context.Background(), v.ID, false); err != nil || text.calls != 1 {
		t.Fatalf("err %v, buscas %d", err, text.calls)
	}
}

func TestRunStructuringFailures(t *testing.T) {
	temp := &app.TextFetchError{Failure: domain.FailureNetwork, Temporary: true, Err: errors.New("timeout")}
	for _, tt := range []struct {
		name      string
		text      *fakeText
		last      bool
		wantErr   bool
		wantState domain.StructuringState
		wantFail  domain.StructuringFailure
	}{
		{"escaneado", &fakeText{pages: []string{" \n", ""}}, false, false, domain.StructuringManual, domain.FailureNoText},
		{"não achou", &fakeText{err: &app.TextFetchError{Failure: domain.FailureNotFound}}, false, false,
			domain.StructuringManual, domain.FailureNotFound},
		{"rede, tenta de novo", &fakeText{err: temp}, false, true, domain.StructuringFetching, domain.FailureNone},
		{"rede, última tentativa", &fakeText{err: temp}, true, false, domain.StructuringManual, domain.FailureNetwork},
		{"texto enorme", &fakeText{pages: []string{strings.Repeat("a", domain.MaxSourceTextLen+1)}}, false, false,
			domain.StructuringManual, domain.FailureTooLarge},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv()
			v := e.importWithURL(t, camaraPDF)
			_, err := e.run(tt.text).Execute(context.Background(), v.ID, tt.last)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			s, _ := e.w.StructuringOf(v.ID)
			if s.State != tt.wantState || s.Failure != tt.wantFail || len(e.w.ProvisionsOf(v.ID)) != 0 {
				t.Fatalf("estruturação = %+v", s)
			}
		})
	}
}

func TestRunStructuringLeavesManualAlone(t *testing.T) {
	e := newEnv()
	v := e.importWithURL(t, "")
	text := &fakeText{pages: []string{pecText}}
	if _, err := e.run(text).Execute(context.Background(), v.ID, false); err != nil || text.calls != 0 {
		t.Fatalf("err %v, buscas %d", err, text.calls)
	}
	if _, err := e.run(text).Execute(context.Background(), "nao-existe", false); !errors.Is(err, domain.ErrStructuringNotFound) {
		t.Fatalf("err = %v", err)
	}
}
