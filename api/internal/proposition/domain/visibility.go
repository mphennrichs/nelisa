package domain

import "time"

// Regra de visibilidade da parte pública (CONTEXT.md, "Revisão"):
//   - "criado" (draft) não existe para o Leitor;
//   - "em revisão" (in_review) aparece como "em análise", só com o que não é
//     produzido pelo Nelisa (dados oficiais, Tramitação, fontes);
//   - "pronto" (ready) aparece completo.
//
// Toda rota /api/public passa por aqui; nenhuma outra camada decide.

// PublicStates são os estados que o Leitor vê.
func PublicStates() []ReviewState { return []ReviewState{ReviewInReview, ReviewReady} }

// IsPublic diz se o Leitor vê uma Proposição neste estado.
func (s ReviewState) IsPublic() bool {
	for _, p := range PublicStates() {
		if s == p {
			return true
		}
	}
	return false
}

// PublicSource é um Vínculo de fonte como o Leitor o vê: a Casa e o link
// para conferir. Só vínculos confirmados; sugestões nunca saem.
type PublicSource struct {
	Chamber    Chamber
	SourceName string
	SourceURL  string
}

// PublicRef aponta outra Proposição (principal ou apensada). Slug vazio
// quando ela não é pública: o nome oficial é dado oficial, a página não.
type PublicRef struct {
	Name string
	Slug string
}

// PublicProposition é tudo o que o Leitor pode ver de uma Proposição.
// Não tem estado de Revisão, quem confirmou vínculos, erros de
// Sincronização nem nada interno: o que não está aqui não tem como vazar.
type PublicProposition struct {
	Slug            string
	Name            string
	Kind            string
	Number          int
	Year            int
	Sphere          Sphere
	OriginChamber   Chamber
	OfficialSummary string
	PresentedAt     *time.Time
	OfficialTextURL string
	Authors         []string
	Status          string
	Superseded      bool
	AttachedTo      *PublicRef
	Attached        []PublicRef
	Sources         []PublicSource
	// UnderReview: "em análise". Enquanto true, só dados oficiais.
	UnderReview bool
	UpdatedAt   time.Time
}

// PublicView aplica a regra de visibilidade. ok é false para o que o
// Leitor não pode ver (o chamador responde 404, sem dizer que existe).
// principal é a Proposição a que p está apensada (nil se não há ou não
// está cadastrada); attached são as apensadas a p.
func PublicView(p Proposition, links []SourceLink, principal *Proposition, attached []Proposition) (PublicProposition, bool) {
	if !p.ReviewState.IsPublic() {
		return PublicProposition{}, false
	}
	out := PublicProposition{
		Slug: p.Slug, Name: p.Name(), Kind: p.Kind, Number: p.Number, Year: p.Year,
		Sphere: p.Sphere, OriginChamber: p.OriginChamber,
		OfficialSummary: p.OfficialSummary, PresentedAt: p.PresentedAt, OfficialTextURL: p.OfficialTextURL,
		Authors: append([]string(nil), p.Authors...), Status: p.Status, Superseded: p.Superseded,
		UnderReview: p.ReviewState == ReviewInReview, UpdatedAt: p.UpdatedAt,
		Sources: []PublicSource{}, Attached: []PublicRef{},
	}
	switch {
	case principal != nil:
		out.AttachedTo = publicRef(*principal)
	case p.AttachedToName != "":
		out.AttachedTo = &PublicRef{Name: p.AttachedToName}
	}
	for _, a := range attached {
		if a.ReviewState.IsPublic() {
			out.Attached = append(out.Attached, *publicRef(a))
		}
	}
	for _, l := range links {
		if l.PropositionID == p.ID && l.State == LinkConfirmed {
			out.Sources = append(out.Sources, PublicSource{Chamber: l.Chamber, SourceName: l.SourceName, SourceURL: l.SourceURL})
		}
	}
	return out, true
}

func publicRef(p Proposition) *PublicRef {
	r := &PublicRef{Name: p.Name()}
	if p.ReviewState.IsPublic() {
		r.Slug = p.Slug
	}
	return r
}
