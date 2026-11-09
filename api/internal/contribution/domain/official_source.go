package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultOfficialSuffixes são aceitos como Fonte oficial sem cadastro
// (CONTEXT.md, "Fonte oficial").
var DefaultOfficialSuffixes = []string{"leg.br", "gov.br"}

// MaxSourceNameLen limita o nome da Casa ou órgão.
const MaxSourceNameLen = 200

var (
	ErrOfficialSourceNotFound = errors.New("contribution: fonte oficial não encontrada")
	// ErrOfficialSourceExists: o domínio já é Fonte oficial (cadastrado ou
	// coberto por .leg.br/.gov.br ou por um domínio cadastrado acima dele).
	ErrOfficialSourceExists = errors.New("contribution: domínio já é fonte oficial")
	// ErrInvalidSourceName: nome da Casa ou órgão vazio ou longo demais.
	ErrInvalidSourceName = errors.New("contribution: informe o nome da Casa ou órgão")
)

// OfficialSource é um domínio cadastrado como Fonte oficial.
type OfficialSource struct {
	ID        string
	Domain    string
	Name      string
	Sphere    Sphere
	CreatedBy string
	CreatedAt time.Time
}

// NewOfficialSource valida e monta o cadastro.
func NewOfficialSource(id, domain, name string, sphere Sphere, by string, now time.Time) (OfficialSource, error) {
	d, err := NormalizeDomain(domain)
	if err != nil {
		return OfficialSource{}, err
	}
	n, err := normalizeName(name)
	if err != nil {
		return OfficialSource{}, err
	}
	if err := sphere.Validate(); err != nil {
		return OfficialSource{}, err
	}
	return OfficialSource{ID: id, Domain: d, Name: n, Sphere: sphere, CreatedBy: by, CreatedAt: now}, nil
}

func normalizeName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || utf8.RuneCountInString(s) > MaxSourceNameLen {
		return "", ErrInvalidSourceName
	}
	return s, nil
}

// IsOfficial diz se host (normalizado) é Fonte oficial: termina em
// .leg.br/.gov.br ou está num domínio cadastrado.
func IsOfficial(host string, registered []string) bool {
	for _, s := range DefaultOfficialSuffixes {
		if InDomain(host, s) {
			return true
		}
	}
	for _, d := range registered {
		if InDomain(host, d) {
			return true
		}
	}
	return false
}
