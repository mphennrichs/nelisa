// Package structuring converte o texto de uma Versão em Dispositivos
// (Estruturação, ADR 0002). É Go puro: recebe texto, devolve a árvore. A
// busca e a extração do PDF ficam fora daqui.
package structuring

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	digitsRe  = regexp.MustCompile(`\d+`)
	spacesRe  = regexp.MustCompile(`[ \t\x{00A0}\x{2007}\x{202F}]+`)
	pageNumRe = regexp.MustCompile(`^\s*\d{1,3}\s*$`)
)

// Clean junta as páginas extraídas de um PDF num texto só, sem cabeçalho e
// rodapé repetidos, com espaços normalizados e palavras hifenizadas no fim
// da linha reunidas. O resultado é o texto que o Editor vê e corrige.
func Clean(pages []string) string {
	pageLines := make([][]string, len(pages))
	for i, p := range pages {
		for _, l := range strings.Split(strings.ReplaceAll(p, "\r", ""), "\n") {
			l = strings.TrimSpace(spacesRe.ReplaceAllString(l, " "))
			if l != "" {
				pageLines[i] = append(pageLines[i], l)
			}
		}
	}

	repeated := repeatedLines(pageLines)
	var lines []string
	for _, pl := range pageLines {
		for _, l := range pl {
			if repeated[repeatKey(l)] || pageNumRe.MatchString(l) {
				continue
			}
			lines = append(lines, l)
		}
	}
	return strings.Join(joinHyphenated(lines), "\n")
}

// repeatKey ignora números, para "Página 2 de 7" e "[3 de 8]" contarem
// como a mesma linha.
func repeatKey(l string) string {
	return digitsRe.ReplaceAllString(strings.ToLower(l), "#")
}

// repeatedLines acha as linhas que aparecem em mais da metade das páginas
// (no mínimo duas): cabeçalho, rodapé, carimbo de assinatura.
func repeatedLines(pageLines [][]string) map[string]bool {
	out := map[string]bool{}
	if len(pageLines) < 2 {
		return out
	}
	count := map[string]int{}
	for _, pl := range pageLines {
		seen := map[string]bool{}
		for _, l := range pl {
			k := repeatKey(l)
			if !seen[k] {
				seen[k] = true
				count[k]++
			}
		}
	}
	min := len(pageLines)/2 + 1
	if min < 2 {
		min = 2
	}
	for k, n := range count {
		if n >= min {
			out[k] = true
		}
	}
	return out
}

// joinHyphenated reúne "regula-" + "mentação" quando a linha seguinte
// continua a palavra em minúscula.
func joinHyphenated(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		for i+1 < len(lines) && endsWithHyphenatedWord(l) && startsLower(lines[i+1]) {
			next := lines[i+1]
			word, rest, _ := strings.Cut(next, " ")
			l = strings.TrimSuffix(l, "-") + word
			if rest != "" {
				lines[i+1] = rest
				break
			}
			i++
		}
		out = append(out, l)
	}
	return out
}

func endsWithHyphenatedWord(l string) bool {
	r := []rune(l)
	return len(r) >= 3 && r[len(r)-1] == '-' && unicode.IsLetter(r[len(r)-2])
}

func startsLower(l string) bool {
	for _, r := range l {
		return unicode.IsLower(r)
	}
	return false
}
