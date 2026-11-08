package structuring

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Kind é o tipo de Dispositivo (LC 95/98), mais ementa, preâmbulo,
// agrupamento (Capítulo, Seção…) e justificação.
type Kind string

const (
	KindEmenta        Kind = "ementa"
	KindPreamble      Kind = "preamble"
	KindHeading       Kind = "heading"
	KindArticle       Kind = "article"
	KindParagraph     Kind = "paragraph"
	KindItem          Kind = "item"    // inciso
	KindPoint         Kind = "point"   // alínea
	KindSubitem       Kind = "subitem" // item de alínea
	KindJustification Kind = "justification"
)

// Commentable diz se o Leitor pode ver Comentário preso a este tipo.
// Agrupamentos (Capítulo, Seção) são só títulos.
func (k Kind) Commentable() bool { return k != KindHeading }

// Provision é um Dispositivo da Versão. Address é a chave estável dentro da
// Versão ("art-3.par-2.inc-4"); Label é o que se mostra ("Art. 3º, § 2º,
// inciso IV"); Text é o texto como está na fonte, com o marcador ("Art. 3º
// …"). Parent é o Address do Dispositivo acima ("" no topo).
type Provision struct {
	Address string
	Parent  string
	Kind    Kind
	Label   string
	Text    string
	Depth   int
}

// Warning avisa o Editor de algo que o parser não conseguiu resolver
// sozinho. Line é a linha (1-based) do texto onde aconteceu, 0 se não houver.
type Warning struct {
	Code    string
	Line    int
	Address string
}

const (
	WarnNoEmenta          = "no_ementa"
	WarnNoArticles        = "no_articles"
	WarnDuplicateAddress  = "duplicate_address"
	WarnArticleOutOfOrder = "article_out_of_order"
	WarnArticleGap        = "article_gap" // pulou número (Art. 1º, Art. 3º)
	WarnOrphan            = "orphan"      // inciso fora de artigo, alínea fora de inciso…
	WarnUnclosedQuote     = "unclosed_quote"
)

// Result é a árvore em ordem de leitura.
type Result struct {
	Provisions []Provision
	Warnings   []Warning
}

var (
	articleRe   = regexp.MustCompile(`^Art\.?\s*0*(\d+)\s*(?:º|°|o\b|ª)?\s*(-\s*[A-Za-z])?\s*[.:–—-]?\s`)
	paragraphRe = regexp.MustCompile(`^§\s*0*(\d+)\s*(?:º|°|o\b)?\s*(-\s*[A-Za-z])?\s*[.:–—-]?\s`)
	soleParRe   = regexp.MustCompile(`(?i)^par[áa]grafo\s+[úu]nico\b`)
	itemRe      = regexp.MustCompile(`^([IVXLCDM]+)(\s*-\s*[A-Z])?\s*[–—-]\s*\S`)
	pointRe     = regexp.MustCompile(`^([a-z])\)\s`)
	subitemRe   = regexp.MustCompile(`^(\d{1,2})\.\s`)
	headingRe   = regexp.MustCompile(`^(?i:(livro|t[íi]tulo|cap[íi]tulo|se[çc][ãa]o|subse[çc][ãa]o))\s+([IVXLCDM]+|[ÚU]NIC[OA])\s*$`)
	justRe      = regexp.MustCompile(`(?i)^justifica(ç|c)(ã|a)o|^justificativa`)
	enactRe     = regexp.MustCompile(`(?i)\b(decreta|promulga[mn]?)\b`)
	endRe       = regexp.MustCompile(`(?i)^(sala das sess[õo]es|(senado federal|c[âa]mara dos deputados),\s|bras[íi]lia,|legisla[çc][ãa]o citada)`)
	headerRe    = regexp.MustCompile(`(?i)^(\(d[oa]s?\b|gabinete\b|projeto\b|proposta\b|n[º°o.]\s|autoria\b|documentos\b|avulso\b|p[áa]gina\b|-\s|https?://)`)
	quoteOpenRe = regexp.MustCompile(`^["“”]`)
	quoteEndRe  = regexp.MustCompile(`["”]\s*(\((NR|AC)\))?\s*\.?$`)
	sentenceEnd = regexp.MustCompile(`[.;:]\s*$`)
)

// Parse quebra o texto limpo de uma Versão em Dispositivos.
func Parse(text string) Result {
	p := &parser{seen: map[string]int{}}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}

	firstArt := -1
	for i, l := range lines {
		if articleRe.MatchString(l) {
			firstArt = i
			break
		}
	}
	bodyEnd := len(lines)
	for i, l := range lines {
		if justRe.MatchString(l) && isShortHeadingLine(l) && (firstArt < 0 || i > firstArt) {
			bodyEnd = i
			break
		}
	}

	headEnd := firstArt
	if headEnd < 0 {
		headEnd = bodyEnd
		p.warn(WarnNoArticles, 0, "")
	}
	bodyStart := p.head(lines[:headEnd])
	if firstArt >= 0 {
		p.body(lines, bodyStart, bodyEnd)
	}
	if bodyEnd < len(lines) {
		p.justification(lines, bodyEnd)
	}
	return Result{Provisions: p.out, Warnings: p.warnings}
}

type parser struct {
	out      []Provision
	warnings []Warning
	seen     map[string]int

	// pilha do corpo: o último Dispositivo de cada nível
	article, paragraph, item, point string
	headings                        int
	lastArticle                     int
}

func (p *parser) warn(code string, line int, addr string) {
	p.warnings = append(p.warnings, Warning{Code: code, Line: line, Address: addr})
}

func (p *parser) add(pr Provision, line int) {
	if n, dup := p.seen[pr.Address]; dup {
		p.seen[pr.Address] = n + 1
		pr.Address = fmt.Sprintf("%s~%d", pr.Address, n+1)
		p.warn(WarnDuplicateAddress, line, pr.Address)
	} else {
		p.seen[pr.Address] = 1
	}
	p.out = append(p.out, pr)
}

func (p *parser) appendText(l string) {
	last := &p.out[len(p.out)-1]
	last.Text = joinLine(last.Text, l)
}

func joinLine(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}

// head separa o que vem antes do primeiro artigo: cabeçalho (descartado),
// ementa e preâmbulo ("O Congresso Nacional decreta:"). Devolve onde o
// corpo começa: logo depois da fórmula de promulgação (pode haver um
// "CAPÍTULO I" antes do primeiro artigo).
func (p *parser) head(lines []string) int {
	enact := -1
	for i, l := range lines {
		if enactRe.MatchString(l) {
			enact = i
			break
		}
	}
	if enact < 0 {
		// sem fórmula de promulgação: tudo o que não é cabeçalho é ementa
		p.ementa(lines, len(lines))
		return len(lines)
	}
	// a fórmula pode começar linhas antes ("AS MESAS … nos termos … / …
	// promulgam …"): volta enquanto a linha anterior não fecha frase
	start := enact
	for start > 0 && !sentenceEnd.MatchString(lines[start-1]) && !headerRe.MatchString(lines[start-1]) {
		start--
	}
	end := enact
	for end < len(lines)-1 && !sentenceEnd.MatchString(lines[end]) {
		end++
	}
	p.ementa(lines, start)
	p.add(Provision{Address: "preambulo", Kind: KindPreamble, Label: "Preâmbulo",
		Text: strings.Join(lines[start:end+1], " ")}, start+1)
	return end + 1
}

// ementa é o último bloco antes de end que não é cabeçalho.
func (p *parser) ementa(lines []string, end int) {
	start := end
	for start > 0 && !isHeader(lines[start-1]) && end-start < 15 {
		start--
	}
	if start == end {
		p.warn(WarnNoEmenta, 0, "")
		return
	}
	p.add(Provision{Address: "ementa", Kind: KindEmenta, Label: "Ementa",
		Text: strings.Join(lines[start:end], " ")}, start+1)
}

// isHeader reconhece linhas de cabeçalho: tudo maiúsculo ("CÂMARA DOS
// DEPUTADOS", "C Â M A R A …"), "(Do Sr. …)", "PROPOSTA DE EMENDA …".
func isHeader(l string) bool {
	if headerRe.MatchString(l) {
		return true
	}
	hasLetter := false
	for _, r := range l {
		if r >= 'a' && r <= 'z' || r >= 'à' && r <= 'ÿ' {
			return false
		}
		if r >= 'A' && r <= 'Z' || r >= 'À' && r <= 'Þ' {
			hasLetter = true
		}
	}
	return hasLetter
}

func isShortHeadingLine(l string) bool { return len([]rune(l)) <= 40 }

func (p *parser) body(lines []string, from, to int) {
	quoted := false
	quoteLine := 0
	pendingHeading := false
	for i := from; i < to; i++ {
		l := lines[i]
		ln := i + 1
		if l == "" {
			continue
		}
		if endRe.MatchString(l) {
			return
		}
		if quoted {
			p.appendText(l)
			if quoteEndRe.MatchString(l) {
				quoted = false
			}
			continue
		}
		if quoteOpenRe.MatchString(l) && len(p.out) > 0 {
			p.appendText(l)
			if !quoteEndRe.MatchString(strings.TrimLeft(l, `"“”`)) {
				quoted, quoteLine = true, ln
			}
			continue
		}
		if pendingHeading && !p.isStructural(l) {
			p.appendText(l)
			pendingHeading = false
			continue
		}
		pendingHeading = false

		switch {
		case articleRe.MatchString(l):
			m := articleRe.FindStringSubmatch(l)
			n, _ := strconv.Atoi(m[1])
			suf := suffix(m[2])
			if n < p.lastArticle || (n == p.lastArticle && suf == "") {
				p.warn(WarnArticleOutOfOrder, ln, "")
			} else if n > p.lastArticle+1 {
				p.warn(WarnArticleGap, ln, "")
			}
			p.lastArticle = n
			addr := "art-" + m[1] + suf
			p.add(Provision{Address: addr, Kind: KindArticle,
				Label: "Art. " + ordinal(n) + upperSuffix(suf), Text: l, Depth: 0}, ln)
			p.article, p.paragraph, p.item, p.point = p.out[len(p.out)-1].Address, "", "", ""

		case paragraphRe.MatchString(l) || soleParRe.MatchString(l):
			if p.article == "" {
				p.orphan(l, ln)
				continue
			}
			var addr, label string
			if m := paragraphRe.FindStringSubmatch(l); m != nil {
				n, _ := strconv.Atoi(m[1])
				suf := suffix(m[2])
				addr, label = "par-"+m[1]+suf, "§ "+ordinal(n)+upperSuffix(suf)
			} else {
				addr, label = "par-unico", "parágrafo único"
			}
			parent := p.article
			p.add(Provision{Address: parent + "." + addr, Parent: parent, Kind: KindParagraph,
				Label: p.labelOf(parent) + ", " + label, Text: l, Depth: 1}, ln)
			p.paragraph, p.item, p.point = p.out[len(p.out)-1].Address, "", ""

		case itemRe.MatchString(l) && validRoman(itemRe.FindStringSubmatch(l)[1]):
			parent := p.paragraph
			if parent == "" {
				parent = p.article
			}
			if parent == "" {
				p.orphan(l, ln)
				continue
			}
			m := itemRe.FindStringSubmatch(l)
			n := romanToInt(m[1])
			suf := suffix(m[2])
			p.add(Provision{Address: parent + ".inc-" + strconv.Itoa(n) + suf, Parent: parent, Kind: KindItem,
				Label: p.labelOf(parent) + ", inciso " + m[1] + upperSuffix(suf), Text: l, Depth: p.depthOf(parent) + 1}, ln)
			p.item, p.point = p.out[len(p.out)-1].Address, ""

		case pointRe.MatchString(l):
			if p.item == "" {
				p.orphan(l, ln)
				continue
			}
			m := pointRe.FindStringSubmatch(l)
			parent := p.item
			p.add(Provision{Address: parent + ".ali-" + m[1], Parent: parent, Kind: KindPoint,
				Label: p.labelOf(parent) + ", alínea " + m[1], Text: l, Depth: p.depthOf(parent) + 1}, ln)
			p.point = p.out[len(p.out)-1].Address

		case p.point != "" && subitemRe.MatchString(l):
			m := subitemRe.FindStringSubmatch(l)
			parent := p.point
			p.add(Provision{Address: parent + ".item-" + m[1], Parent: parent, Kind: KindSubitem,
				Label: p.labelOf(parent) + ", item " + m[1], Text: l, Depth: p.depthOf(parent) + 1}, ln)

		case headingRe.MatchString(l):
			p.headings++
			p.add(Provision{Address: "h-" + strconv.Itoa(p.headings), Kind: KindHeading,
				Label: l, Text: l}, ln)
			p.article, p.paragraph, p.item, p.point = "", "", "", ""
			pendingHeading = true

		default:
			p.appendText(l)
		}
	}
	if quoted {
		p.warn(WarnUnclosedQuote, quoteLine, "")
	}
}

func (p *parser) isStructural(l string) bool {
	return articleRe.MatchString(l) || paragraphRe.MatchString(l) || soleParRe.MatchString(l) ||
		itemRe.MatchString(l) || pointRe.MatchString(l) || headingRe.MatchString(l)
}

// orphan: marcador sem pai (inciso antes de qualquer artigo, alínea sem
// inciso). Vira continuação do Dispositivo anterior e avisa o Editor.
func (p *parser) orphan(l string, ln int) {
	p.warn(WarnOrphan, ln, "")
	if len(p.out) > 0 {
		p.appendText(l)
	}
}

func (p *parser) justification(lines []string, from int) {
	var paras []string
	cur := ""
	for i := from + 1; i < len(lines); i++ {
		l := lines[i]
		if endRe.MatchString(l) {
			break
		}
		if l == "" {
			continue
		}
		cur = joinLine(cur, l)
		if sentenceEnd.MatchString(l) && len([]rune(l)) < 70 {
			// linha curta que fecha frase: fim de parágrafo
			paras = append(paras, cur)
			cur = ""
		}
	}
	if cur != "" {
		paras = append(paras, cur)
	}
	p.add(Provision{Address: "justificacao", Kind: KindJustification, Label: "Justificação",
		Text: strings.Join(paras, "\n\n")}, from+1)
}

func (p *parser) labelOf(addr string) string {
	for i := len(p.out) - 1; i >= 0; i-- {
		if p.out[i].Address == addr {
			return p.out[i].Label
		}
	}
	return ""
}

func (p *parser) depthOf(addr string) int {
	for i := len(p.out) - 1; i >= 0; i-- {
		if p.out[i].Address == addr {
			return p.out[i].Depth
		}
	}
	return 0
}

// suffix normaliza "-A" de "Art. 3º-A" para "a" no endereço.
func suffix(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "-"))
	return strings.ToLower(strings.TrimSpace(s))
}

func upperSuffix(s string) string {
	if s == "" {
		return ""
	}
	return "-" + strings.ToUpper(s)
}

// ordinal segue a LC 95/98: ordinal até o nono, cardinal a partir do décimo.
func ordinal(n int) string {
	if n <= 9 {
		return strconv.Itoa(n) + "º"
	}
	return strconv.Itoa(n)
}

var romanVal = map[rune]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}

func romanToInt(s string) int {
	total := 0
	for i, r := range s {
		v := romanVal[r]
		if i+1 < len(s) && v < romanVal[rune(s[i+1])] {
			total -= v
		} else {
			total += v
		}
	}
	return total
}

// validRoman recusa siglas que parecem romanos ("DC – ", "CLI – ") quando
// a conversão de volta não bate.
func validRoman(s string) bool {
	n := romanToInt(s)
	return n > 0 && n < 400 && intToRoman(n) == s
}

func intToRoman(n int) string {
	vals := []int{100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}
