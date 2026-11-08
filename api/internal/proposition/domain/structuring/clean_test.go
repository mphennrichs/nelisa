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
