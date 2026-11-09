package structuring

import (
	"os"
	"strings"
	"testing"
)

func addrs(r Result) []string {
	var out []string
	for _, p := range r.Provisions {
		out = append(out, p.Address)
	}
	return out
}

func find(t *testing.T, r Result, addr string) Provision {
	t.Helper()
	for _, p := range r.Provisions {
		if p.Address == addr {
			return p
		}
	}
	t.Fatalf("sem %q em %v", addr, addrs(r))
	return Provision{}
}

func warnCodes(r Result) []string {
	var out []string
	for _, w := range r.Warnings {
		out = append(out, w.Code)
	}
	return out
}

const sample = `CÂMARA DOS DEPUTADOS
PROJETO DE LEI Nº , DE 2026
(Do Sr. Fulano)
Dispõe sobre a proteção de
nascentes urbanas.
O Congresso Nacional decreta:
CAPÍTULO I
DISPOSIÇÕES GERAIS
Art. 1º Esta Lei protege nascentes
urbanas.
Parágrafo único. Aplica-se a todos os Municípios.
Art. 2º São deveres do Município:
I – mapear as nascentes;
II – fiscalizar:
a) obras próximas;
b) descarte de resíduos, em especial:
1. entulho;
2. óleo;
III – informar a população.
§ 1º O mapa é público.
§ 2º O mapa é revisto a cada dois anos.
Art. 2º-A A Lei nº 1.234, de 2000, passa a vigorar com:
“Art. 5º ..................
§ 3º Fica proibido aterrar nascente.” (NR)
Art. 10. Esta Lei entra em vigor na data de sua publicação.
JUSTIFICAÇÃO
Nascentes urbanas somem todo ano.
O projeto cria deveres claros
para os Municípios, sem custo novo.
Sala das Sessões, em de de 2026.
Deputado Fulano`

func TestParseStructure(t *testing.T) {
	r := Parse(sample)
	want := []string{
		"ementa", "preambulo", "h-1",
		"art-1", "art-1.par-unico",
		"art-2", "art-2.inc-1", "art-2.inc-2", "art-2.inc-2.ali-a", "art-2.inc-2.ali-b",
		"art-2.inc-2.ali-b.item-1", "art-2.inc-2.ali-b.item-2", "art-2.inc-3",
		"art-2.par-1", "art-2.par-2",
		"art-2a", "art-10", "justificacao",
	}
	if got := addrs(r); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("endereços\n got %v\nwant %v", got, want)
	}

	cases := map[string]struct{ label, text string }{
		"ementa":                   {"Ementa", "Dispõe sobre a proteção de nascentes urbanas."},
		"preambulo":                {"Preâmbulo", "O Congresso Nacional decreta:"},
		"h-1":                      {"CAPÍTULO I", "CAPÍTULO I DISPOSIÇÕES GERAIS"},
		"art-1":                    {"Art. 1º", "Art. 1º Esta Lei protege nascentes urbanas."},
		"art-1.par-unico":          {"Art. 1º, parágrafo único", "Parágrafo único. Aplica-se a todos os Municípios."},
		"art-2.inc-2.ali-b.item-1": {"Art. 2º, inciso II, alínea b, item 1", "1. entulho;"},
		"art-2.par-2":              {"Art. 2º, § 2º", "§ 2º O mapa é revisto a cada dois anos."},
		"art-10":                   {"Art. 10", "Art. 10. Esta Lei entra em vigor na data de sua publicação."},
	}
	for addr, c := range cases {
		p := find(t, r, addr)
		if p.Label != c.label || p.Text != c.text {
			t.Errorf("%s: label %q texto %q; quer %q %q", addr, p.Label, p.Text, c.label, c.text)
		}
	}

	// texto citado entre aspas fica dentro do artigo que cita
	a := find(t, r, "art-2a")
	if a.Label != "Art. 2º-A" || !strings.Contains(a.Text, "§ 3º Fica proibido aterrar nascente.” (NR)") {
		t.Errorf("art-2a: %+v", a)
	}
	if p := find(t, r, "art-2.inc-2.ali-b.item-2"); p.Depth != 3 || p.Parent != "art-2.inc-2.ali-b" {
		t.Errorf("profundidade/pai: %+v", p)
	}
	if find(t, r, "h-1").Kind.Commentable() {
		t.Error("agrupamento não recebe Comentário")
	}

	j := find(t, r, "justificacao")
	if strings.Contains(j.Text, "Sala das Sessões") || strings.Contains(j.Text, "Fulano") {
		t.Errorf("justificação levou a assinatura: %q", j.Text)
	}
	if !strings.Contains(j.Text, "Nascentes urbanas somem todo ano.\n\nO projeto") {
		t.Errorf("parágrafos da justificação: %q", j.Text)
	}
	if len(r.Warnings) != 1 || r.Warnings[0].Code != WarnArticleGap {
		t.Errorf("avisos: %v (só o pulo do 2-A para o 10)", r.Warnings)
	}
}

func TestParseWarnings(t *testing.T) {
	tests := []struct {
		name, text string
		want       []string
	}{
		{"sem artigos", "Dispõe sobre algo.\nTexto solto.", []string{WarnNoArticles}},
		{"sem ementa", "O Congresso Nacional decreta:\nArt. 1º Algo.", []string{WarnNoEmenta}},
		{"endereço repetido", "Ementa.\nO Congresso Nacional decreta:\nArt. 1º A.\nArt. 1º B.",
			[]string{WarnArticleOutOfOrder, WarnDuplicateAddress}},
		{"inciso fora de artigo", "Ementa.\nO Congresso Nacional decreta:\nXIII – solto;\nArt. 1º A.",
			[]string{WarnOrphan}},
		{"alínea sem inciso", "Ementa.\nO Congresso Nacional decreta:\nArt. 1º A:\na) solta.",
			[]string{WarnOrphan}},
		{"aspas sem fechar", "Ementa.\nO Congresso Nacional decreta:\nArt. 1º Altera:\n“Art. 5º texto",
			[]string{WarnUnclosedQuote}},
		{"artigo fora de ordem", "Ementa.\nO Congresso Nacional decreta:\nArt. 2º A.\nArt. 1º B.",
			[]string{WarnArticleGap, WarnArticleOutOfOrder}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.text)
			if got := warnCodes(r); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("avisos %v, quer %v", got, tt.want)
			}
		})
	}
	r := Parse("Ementa.\nO Congresso Nacional decreta:\nArt. 1º A.\nArt. 1º B.")
	if got := addrs(r); strings.Join(got, " ") != "ementa preambulo art-1 art-1~2" {
		t.Errorf("repetido ganha sufixo: %v", got)
	}
}

func TestParseNotRomanOrHeadingInText(t *testing.T) {
	// "Seção IV deste Capítulo" no meio de um parágrafo não é agrupamento;
	// "DC – " não é inciso.
	r := Parse(`Ementa.
O Congresso Nacional decreta:
Art. 1º Texto.
§ 1º Conforme a
Seção IV deste Capítulo, vale.
§ 2º Outro.
Art. 2º Lista:
I – primeiro;
DC – não é inciso.`)
	if got := strings.Join(addrs(r), " "); got != "ementa preambulo art-1 art-1.par-1 art-1.par-2 art-2 art-2.inc-1" {
		t.Errorf("endereços: %s", got)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("avisos: %v", r.Warnings)
	}
}

// Textos reais extraídos dos PDFs oficiais (spike de 2026-11-08).
func TestParseRealDocuments(t *testing.T) {
	tests := []struct {
		file      string
		articles  int
		mustHave  []string
		warnCodes []string
	}{
		{"camara_pec_221_2019", 1, []string{"ementa", "preambulo", "art-3", "justificacao"},
			[]string{WarnOrphan, WarnArticleGap}}, // o PDF original pula do preâmbulo ao Art. 3º
		{"senado_pec_221_2019", 10, []string{"ementa", "preambulo", "art-2.par-unico", "art-10.inc-2"}, nil},
		{"camara_pec_8_2025", 2, []string{"ementa", "preambulo", "art-1", "art-2", "justificacao"}, nil},
		{"camara_pec_9_2026", 2, []string{"ementa", "preambulo", "art-1", "art-2", "justificacao"}, nil},
		{"camara_pl_2338_2023", 79, []string{"ementa", "h-1", "art-1.par-1.inc-4", "art-80.par-2.inc-2.ali-b"},
			[]string{WarnArticleGap}}, // não há Art. 31 no texto
		{"senado_pl_2338_2023", 79, []string{"ementa", "h-1", "art-1.par-1.inc-4", "art-80.par-2.inc-2.ali-b"},
			[]string{WarnArticleGap}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			b, err := os.ReadFile("testdata/" + tt.file + ".pages.txt")
			if err != nil {
				t.Fatal(err)
			}
			r := Parse(Clean(strings.Split(string(b), "\f")))
			n := 0
			for _, p := range r.Provisions {
				if p.Kind == KindArticle {
					n++
				}
				if strings.Contains(p.Text, "Assinado eletronicamente") || strings.Contains(p.Text, "Apresentação:") ||
					strings.Contains(p.Text, "*CD") || strings.Contains(p.Text, "Anexo IV") || strings.Contains(p.Text, "LexEdit") {
					t.Errorf("%s levou carimbo de página: %.120q", p.Address, p.Text)
				}
			}
			if n != tt.articles {
				t.Errorf("%d artigos, quer %d", n, tt.articles)
			}
			for _, a := range tt.mustHave {
				find(t, r, a)
			}
			if got := warnCodes(r); strings.Join(got, ",") != strings.Join(tt.warnCodes, ",") {
				t.Errorf("avisos %v, quer %v", r.Warnings, tt.warnCodes)
			}
		})
	}
}
