// Package domain tem as regras da Proposição (CONTEXT.md, ADR 0006): o nome
// oficial, o estado de Revisão, os dados oficiais vindos das fontes, a
// Apensação, o Vínculo de fonte e a regra de visibilidade da parte
// pública. Só Go padrão.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrPropositionNotFound = errors.New("proposition: proposição não encontrada")
	// ErrPropositionExists: já há uma Proposição com a mesma Casa
	// iniciadora, Tipo, número e ano.
	ErrPropositionExists = errors.New("proposition: proposição já cadastrada")
	ErrSlugTaken         = errors.New("proposition: identificador público já usado")
	ErrInvalidKind       = errors.New("proposition: tipo inválido (sigla em maiúsculas, até 10 letras)")
	ErrInvalidNumber     = errors.New("proposition: número inválido")
	ErrInvalidYear       = errors.New("proposition: ano inválido")
	ErrInvalidChamber    = errors.New("proposition: casa legislativa inválida")
	ErrInvalidSphere     = errors.New("proposition: esfera inválida")
	ErrInvalidName       = errors.New("proposition: nome oficial inválido (ex.: PEC 221/2019)")
	// ErrInvalidTransition: a mudança de estado de Revisão não parte do
	// estado certo.
	ErrInvalidTransition = errors.New("proposition: mudança de estado de revisão inválida")
)

// Chamber é o código de uma Casa legislativa. Na v1 só as Casas com API.
type Chamber string

const (
	ChamberCamara Chamber = "camara"
	ChamberSenado Chamber = "senado"
)

// ParseChamber aceita só as Casas conhecidas.
func ParseChamber(s string) (Chamber, error) {
	switch Chamber(s) {
	case ChamberCamara, ChamberSenado:
		return Chamber(s), nil
	}
	return "", ErrInvalidChamber
}

// Sphere é a Esfera da Proposição.
type Sphere string

const (
	SphereFederal   Sphere = "federal"
	SphereState     Sphere = "state"
	SphereMunicipal Sphere = "municipal"
)

// SphereOf deriva a Esfera da Casa: a Casa define a Esfera (CONTEXT.md).
func SphereOf(c Chamber) (Sphere, error) {
	switch c {
	case ChamberCamara, ChamberSenado:
		return SphereFederal, nil
	}
	return "", ErrInvalidSphere
}

// ReviewState é o estado de Revisão: criado → em revisão → pronto.
type ReviewState string

const (
	ReviewDraft    ReviewState = "draft"
	ReviewInReview ReviewState = "in_review"
	ReviewReady    ReviewState = "ready"
)

var kindPattern = regexp.MustCompile(`^[A-Z]{1,10}$`)

// ParseKind normaliza a sigla do Tipo (PL, PEC, PLP, MPV...).
func ParseKind(s string) (string, error) {
	k := strings.ToUpper(strings.TrimSpace(s))
	if !kindPattern.MatchString(k) {
		return "", ErrInvalidKind
	}
	return k, nil
}

// ValidateNumberYear confere número e ano do nome oficial.
func ValidateNumberYear(number, year int) error {
	if number <= 0 || number > 1_000_000 {
		return ErrInvalidNumber
	}
	if year < 1800 || year > 9999 {
		return ErrInvalidYear
	}
	return nil
}

// OfficialName monta o nome oficial ("PEC 221/2019").
func OfficialName(kind string, number, year int) string {
	return fmt.Sprintf("%s %d/%d", kind, number, year)
}

var namePattern = regexp.MustCompile(`^([A-Za-z]{1,10})\s+n?º?\s*(\d{1,7})\s*/\s*(\d{4})$`)

// ParseOfficialName lê "PEC 221/2019" (como as fontes escrevem a
// Apensação) em Tipo, número e ano.
func ParseOfficialName(s string) (kind string, number, year int, err error) {
	m := namePattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", 0, 0, ErrInvalidName
	}
	kind, err = ParseKind(m[1])
	if err != nil {
		return "", 0, 0, ErrInvalidName
	}
	number, _ = strconv.Atoi(m[2])
	year, _ = strconv.Atoi(m[3])
	if ValidateNumberYear(number, year) != nil {
		return "", 0, 0, ErrInvalidName
	}
	return kind, number, year, nil
}

// Slug é o identificador público base ("pec-221-2019").
func Slug(kind string, number, year int) string {
	return fmt.Sprintf("%s-%d-%d", strings.ToLower(kind), number, year)
}

// SlugWithChamber desempata o slug quando Tipo/número/ano já existem com
// outra Casa iniciadora.
func SlugWithChamber(kind string, number, year int, origin Chamber) string {
	return Slug(kind, number, year) + "-" + string(origin)
}

// Limites dos dados oficiais: a fonte é tratada como hostil.
const (
	maxSummaryLen = 10_000
	maxStatusLen  = 1_000
	maxURLLen     = 2_000
	maxAuthors    = 200
	maxAuthorLen  = 300
)

// OfficialData é o que as fontes dizem da Proposição. Nada aqui é
// produzido pelo Nelisa; em "em análise" é tudo o que o Leitor vê.
type OfficialData struct {
	Summary         string
	PresentedAt     *time.Time
	OfficialTextURL string
	Authors         []string
	Status          string
	// AttachedToName é o nome oficial da principal ("" se não apensada).
	AttachedToName string
	Superseded     bool
}

// Clean corta e limpa o que vem da fonte: espaços nas pontas, tamanho
// máximo, URL só http(s), autores vazios fora.
func (d OfficialData) Clean() OfficialData {
	out := OfficialData{
		Summary:        truncate(strings.TrimSpace(d.Summary), maxSummaryLen),
		Status:         truncate(strings.TrimSpace(d.Status), maxStatusLen),
		AttachedToName: truncate(strings.TrimSpace(d.AttachedToName), 100),
	}
	out.Superseded = d.Superseded || IsSupersededStatus(out.Status)
	if d.PresentedAt != nil && !d.PresentedAt.IsZero() {
		t := d.PresentedAt.UTC()
		out.PresentedAt = &t
	}
	if u := strings.TrimSpace(d.OfficialTextURL); len(u) <= maxURLLen && IsHTTPURL(u) {
		out.OfficialTextURL = u
	}
	for _, a := range d.Authors {
		if a = truncate(strings.TrimSpace(a), maxAuthorLen); a != "" && len(out.Authors) < maxAuthors {
			out.Authors = append(out.Authors, a)
		}
	}
	return out
}

// IsHTTPURL aceita só links http(s) absolutos: um "javascript:" vindo da
// fonte nunca chega ao Leitor.
func IsHTTPURL(u string) bool {
	l := strings.ToLower(u)
	return (strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://")) && !strings.ContainsAny(u, " \t\r\n<>\"")
}

// IsSupersededStatus diz se a situação oficial indica Proposição
// prejudicada. As fontes não têm um campo próprio para isso; a situação
// ("Prejudicada", "Declarada prejudicada") é o sinal disponível.
func IsSupersededStatus(status string) bool {
	return strings.Contains(strings.ToLower(status), "prejudicad")
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

// Proposition é a unidade central do Nelisa: uma proposta legislativa com
// um registro só, mesmo passando por várias Casas (ADR 0006).
type Proposition struct {
	ID            string
	Slug          string
	Kind          string
	Number        int
	Year          int
	Sphere        Sphere
	OriginChamber Chamber
	OfficialData
	// StatusAt é a data do evento de Tramitação que sustenta Status.
	StatusAt *time.Time
	// AttachedToID é a principal, quando a Apensação foi resolvida.
	AttachedToID *string
	ReviewState  ReviewState
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewProposition monta uma Proposição recém-importada, em "criado".
func NewProposition(id, slug, kind string, number, year int, origin Chamber, data OfficialData, now time.Time) (Proposition, error) {
	k, err := ParseKind(kind)
	if err != nil {
		return Proposition{}, err
	}
	if err := ValidateNumberYear(number, year); err != nil {
		return Proposition{}, err
	}
	if _, err := ParseChamber(string(origin)); err != nil {
		return Proposition{}, err
	}
	sphere, err := SphereOf(origin)
	if err != nil {
		return Proposition{}, err
	}
	return Proposition{
		ID: id, Slug: slug, Kind: k, Number: number, Year: year, Sphere: sphere, OriginChamber: origin,
		OfficialData: data.Clean(), ReviewState: ReviewDraft, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Name é o nome oficial da Proposição.
func (p Proposition) Name() string { return OfficialName(p.Kind, p.Number, p.Year) }

// SubmitForReview leva de "criado" para "em revisão": a partir daí a parte
// pública mostra os dados oficiais como "em análise". "Pronto" é da fase 2.
func (p *Proposition) SubmitForReview(now time.Time) error {
	if p.ReviewState != ReviewDraft {
		return ErrInvalidTransition
	}
	p.ReviewState = ReviewInReview
	p.UpdatedAt = now
	return nil
}

// SyncUpdate é o que uma Sincronização de uma Casa traz.
type SyncUpdate struct {
	Chamber Chamber
	Data    OfficialData
	// LatestEventAt é a data do evento mais recente da Tramitação nessa
	// Casa (nil se a Casa não tem eventos).
	LatestEventAt *time.Time
	// ReportsAttachment: a fonte desta Casa informa a Apensação, e um
	// AttachedToName vazio quer dizer "não está apensada" (desapensação).
	// Numa fonte que nunca informa, o vazio não quer dizer nada.
	ReportsAttachment bool
}

// ApplySync aplica dados oficiais de uma Casa. Identificação (ementa,
// autores, apresentação, link do Documento integral) vem da Casa
// iniciadora. Situação e Prejudicada vêm da Casa com o evento de
// Tramitação mais recente, que é onde a Proposição está agora. A Apensação
// vem só de fonte que a informa. Nunca mexe no estado de Revisão:
// Tramitação não altera Revisão (CONTEXT.md).
func (p *Proposition) ApplySync(u SyncUpdate, now time.Time) {
	d := u.Data.Clean()
	if u.Chamber == p.OriginChamber {
		p.Summary, p.PresentedAt, p.OfficialTextURL, p.Authors = d.Summary, d.PresentedAt, d.OfficialTextURL, d.Authors
	}
	if u.ReportsAttachment {
		if p.AttachedToName != d.AttachedToName {
			p.AttachedToID = nil // a principal mudou ou deixou de existir: resolve de novo
		}
		p.AttachedToName = d.AttachedToName
	}
	if p.StatusAt == nil || (u.LatestEventAt != nil && !u.LatestEventAt.Before(*p.StatusAt)) {
		p.Status, p.Superseded = d.Status, d.Superseded
		if u.LatestEventAt != nil {
			t := u.LatestEventAt.UTC()
			p.StatusAt = &t
		}
	}
	p.UpdatedAt = now
}

// AttachTo resolve a Apensação para a principal já cadastrada.
func (p *Proposition) AttachTo(principalID string) {
	if principalID == "" || principalID == p.ID {
		p.AttachedToID = nil
		return
	}
	p.AttachedToID = &principalID
}
