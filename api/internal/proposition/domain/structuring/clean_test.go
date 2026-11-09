package structuring

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	pages := []string{
		"CÂMARA DOS DEPUTADOS\r\nArt. 1º Esta Lei regula-\r\nmenta o uso\u00a0 de  dados.\r\nPEC n.9/2026\r\nPágina 1 de 3\r\n",
		"CÂMARA DOS DEPUTADOS\nArt. 2º Guarda-\nChuva fica.\nPEC n.9/2026\nPágina 2 de 3\n 2 \n",
		"CÂMARA DOS DEPUTADOS\nArt. 3º Fim.\nPEC n.9/2026\nPágina 3 de 3\n",
	}
	got := Clean(pages)
	want := "Art. 1º Esta Lei regulamenta\no uso de dados.\nArt. 2º Guarda-\nChuva fica.\nArt. 3º Fim."
	if got != want {
		t.Errorf("Clean\n got %q\nwant %q", got, want)
	}
}

func TestCleanSinglePageKeepsEverything(t *testing.T) {
	got := Clean([]string{"CÂMARA DOS DEPUTADOS\nArt. 1º Texto."})
	if !strings.HasPrefix(got, "CÂMARA DOS DEPUTADOS") {
		t.Errorf("página única não tem como saber o que é cabeçalho: %q", got)
	}
}

func TestCleanKeepsRepeatedProvisionLinesAndDropsStamps(t *testing.T) {
	// "........" e "Parágrafo único." repetidos em 3 páginas são texto, não
	// rodapé; o rodapé do gabinete em 3 de 6 páginas sai; carimbo sai mesmo
	// aparecendo uma vez.
	page := func(extra string) string {
		return "Gabinete da Deputada Fulana\n..........\nParágrafo único. Igual.\n" + extra
	}
	pages := []string{page("Art. 1º Um."), page("Art. 2º Dois."), page("*CD230955946600* PEC n.9/2026\nArt. 3º Três."), "Assinaturas", "Assinaturas", "Assinaturas"}
	got := Clean(pages)
	if strings.Contains(got, "Gabinete") || strings.Contains(got, "*CD") {
		t.Errorf("rodapé ou carimbo ficou: %q", got)
	}
	if strings.Count(got, "..........") != 3 || strings.Count(got, "Parágrafo único. Igual.") != 3 {
		t.Errorf("texto repetido sumiu: %q", got)
	}
}
