package httpsrc

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // o fuso de Brasília precisa existir mesmo em imagem sem tzdata
	"unicode"
	"unicode/utf8"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
)

// Limites contra valores absurdos vindos da fonte.
const (
	MaxTextLen = 100_000 // caracteres de um campo de texto (ementa, despacho)
	MinYear    = 1900
	MaxYear    = 2100
	MaxNumber  = 1_000_000
)

// Brasilia é o fuso usado quando a data da fonte não traz deslocamento.
var Brasilia = mustLoad("America/Sao_Paulo")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// Text limpa um texto da fonte: UTF-8 válido, sem caracteres de controle
// (exceto quebra de linha e tab), sem espaços nas pontas, quebras de linha
// normalizadas para \n. Erra se passar de MaxTextLen.
func Text(field, s string) (string, error) {
	s = strings.ToValidUTF8(s, "�")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == '\r':
			return '\n'
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxTextLen {
		return "", Invalid("%s com mais de %d caracteres", field, MaxTextLen)
	}
	return s, nil
}

// Line é Text para campos de uma linha só: junta todo espaço em branco.
func Line(field, s string) (string, error) {
	s, err := Text(field, s)
	if err != nil {
		return "", err
	}
	return strings.Join(strings.Fields(s), " "), nil
}

var kindRe = regexp.MustCompile(`^[A-Z]{1,12}$`)

// NormalizeQuery valida e normaliza a busca: sigla em maiúsculas, número e
// ano em faixas plausíveis. Busca inválida não pode existir na fonte, então
// vira ErrNotFound.
func NormalizeQuery(q app.SearchQuery) (app.SearchQuery, error) {
	q.Kind = strings.ToUpper(strings.TrimSpace(q.Kind))
	if !kindRe.MatchString(q.Kind) || !ValidNumber(q.Number) || !ValidYear(q.Year) {
		return q, fmt.Errorf("%w: busca inválida %q %d/%d", app.ErrNotFound, q.Kind, q.Number, q.Year)
	}
	return q, nil
}

// Kind valida a sigla do Tipo vinda da fonte.
func Kind(s string) (string, error) {
	k := strings.ToUpper(strings.TrimSpace(s))
	if !kindRe.MatchString(k) {
		return "", Invalid("sigla do tipo %q", s)
	}
	return k, nil
}

// ValidNumber diz se o número da Proposição é plausível.
func ValidNumber(n int) bool { return n > 0 && n <= MaxNumber }

// ValidYear diz se o ano da Proposição é plausível.
func ValidYear(y int) bool { return y >= MinYear && y <= MaxYear }

var idRe = regexp.MustCompile(`^[0-9]{1,15}$`)

// ExternalID valida um ID externo antes de pô-lo numa URL: só dígitos.
// Qualquer outra coisa não existe na fonte.
func ExternalID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if !idRe.MatchString(id) || strings.Trim(id, "0") == "" {
		return "", fmt.Errorf("%w: ID externo inválido %q", app.ErrNotFound, id)
	}
	return id, nil
}

var dateLayouts = []string{
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

// Date interpreta uma data da fonte. Com deslocamento (RFC 3339) usa o dele;
// sem, assume o horário de Brasília. Rejeita datas fora de
// [MinYear, agora + 1 dia].
func Date(field, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		for _, l := range dateLayouts {
			if t, err = time.ParseInLocation(l, s, Brasilia); err == nil {
				break
			}
		}
	}
	if err != nil {
		return time.Time{}, Invalid("%s %q não é uma data", field, s)
	}
	if t.Year() < MinYear || t.After(time.Now().Add(24*time.Hour)) {
		return time.Time{}, Invalid("%s %q fora da faixa plausível", field, s)
	}
	return t, nil
}

// OfficialURL aceita só links http(s) para domínios oficiais (.leg.br ou
// .gov.br); qualquer outra coisa vira "" (o campo é opcional e iria para a
// página pública).
func OfficialURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(h, ".leg.br") && !strings.HasSuffix(h, ".gov.br") {
		return ""
	}
	return u.String()
}

// FlexInt aceita inteiro em JSON como número ou como string ("221",
// "00221") — as duas APIs alternam entre os formatos. null vira 0.
type FlexInt int64

// UnmarshalJSON implementa json.Unmarshaler.
func (f *FlexInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(strings.Trim(string(b), `"`))
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("inteiro inválido %s", b)
	}
	*f = FlexInt(n)
	return nil
}
