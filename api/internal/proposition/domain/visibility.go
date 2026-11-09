package domain

import (
	"sort"
	"time"
)

// Regra de visibilidade da parte pública (CONTEXT.md, "Revisão" e
// "Versão vigente / em análise"):
//   - "criado" (draft) não existe para o Leitor;
//   - sem Versão vigente, aparece como "em análise", só com o que não é
//     produzido pelo Nelisa (dados oficiais, Tramitação, fontes);
//   - com Versão vigente, ganha Resumo, Resumo curto, Temas e Selo dela;
//   - com Versão vigente, também as notas das Erratas aprovadas dela
//     (data, "dizia", "o correto é"), nunca pendentes, recusadas ou
//     retiradas, nem nomes;
//   - com vigente e uma Versão em análise, também o aviso "há texto novo em
//     análise" com o rótulo e o link oficial da Versão nova, e nada mais
//     dela.
//
// Nunca saem: conteúdo da Versão em análise, Autoria editorial, quem
// aprovou, Posse, "a confirmar" e a revisão (trava otimista).
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
	// UnderReview: "em análise" (sem Versão vigente). Enquanto true, só
	// dados oficiais.
	UnderReview bool
	UpdatedAt   time.Time

	// Da Versão vigente; vazios sem ela.
	Summary      string
	ShortSummary string
	Topics       []string
	// ReviewBadge é o Selo: 2 ("revisado por 2 pessoas") ou 1; 0 sem
	// Versão vigente.
	ReviewBadge int
	// PendingVersion é o aviso de texto novo em análise (só com vigente).
	PendingVersion *PublicPendingVersion
	// Errata são as notas das Erratas aprovadas da Versão vigente, da mais
	// nova para a mais antiga. Sem nomes: nem quem propôs nem quem aprovou.
	Errata []PublicErratum
	// AnnotatedText é o Documento comentado da Versão vigente: os
	// Dispositivos na ordem de leitura, com os Comentários. nil sem
	// Estruturação confirmada (o Leitor vê só o link oficial).
	AnnotatedText []PublicProvision
}

// PublicProvision é um Dispositivo como o Leitor o vê, com os Comentários
// (só o texto: sem autor, sem data de edição).
type PublicProvision struct {
	Address  string
	Label    string
	Kind     string
	Text     string
	Depth    int
	Comments []string
}

// PublicErratum é a nota pública de uma Errata aprovada: "Em <data>:
// dizia X; o correto é Y". É tudo o que o Leitor sabe dela.
type PublicErratum struct {
	VersionID  string
	ApprovedAt time.Time
	Said       string
	Correct    string
}

// PublicPendingVersion é tudo o que o Leitor sabe da Versão em análise: o
// rótulo e o link oficial. O conteúdo dela não sai.
type PublicPendingVersion struct {
	Label           string
	OfficialTextURL string
}

// Editorial é o ciclo editorial de uma Proposição, como a camada pública o
// recebe: todas as Versões e os conteúdos disponíveis (a regra escolhe o
// que sai, quem chama não precisa filtrar), os Temas conhecidos e o Selo
// de cada Versão aprovada (decidido pelo módulo review).
type Editorial struct {
	Versions []Version
	Contents []Content
	Topics   map[string]Topic
	Badges   map[string]int
	// Errata são as Erratas aprovadas conhecidas (o módulo review só manda
	// aprovadas); a regra só usa as da Versão vigente.
	Errata []PublicErratum
	// Estruturação, Dispositivos (por Versão) e Comentários conhecidos; a
	// regra só usa os da vigente, confirmada, e Comentários vivos e
	// confirmados.
	Structurings []Structuring
	Provisions   map[string][]Provision
	Comments     []Comment
}

// PublicView aplica a regra de visibilidade. ok é false para o que o
// Leitor não pode ver (o chamador responde 404, sem dizer que existe).
// principal é a Proposição a que p está apensada (nil se não há ou não
// está cadastrada); attached são as apensadas a p.
func PublicView(p Proposition, links []SourceLink, principal *Proposition, attached []Proposition, ed Editorial) (PublicProposition, bool) {
	if !p.ReviewState.IsPublic() {
		return PublicProposition{}, false
	}
	out := PublicProposition{
		Slug: p.Slug, Name: p.Name(), Kind: p.Kind, Number: p.Number, Year: p.Year,
		Sphere: p.Sphere, OriginChamber: p.OriginChamber,
		OfficialSummary: p.OfficialSummary, PresentedAt: p.PresentedAt, OfficialTextURL: p.OfficialTextURL,
		Authors: append([]string(nil), p.Authors...), Status: p.Status, Superseded: p.Superseded,
		UnderReview: true, UpdatedAt: p.UpdatedAt,
		Sources: []PublicSource{}, Attached: []PublicRef{}, Topics: []string{}, Errata: []PublicErratum{},
	}
	applyEditorial(&out, p, ed)
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

// applyEditorial acrescenta o que é da Versão vigente e o aviso da Versão
// em análise. Conteúdo só é lido se for da vigente desta Proposição.
func applyEditorial(out *PublicProposition, p Proposition, ed Editorial) {
	var current, pending *Version
	for i := range ed.Versions {
		v := &ed.Versions[i]
		if v.PropositionID != p.ID {
			continue
		}
		switch v.State {
		case VersionCurrent:
			current = v
		case VersionPending:
			pending = v
		}
	}
	if current == nil {
		return
	}
	var content *Content
	for i := range ed.Contents {
		if ed.Contents[i].VersionID == current.ID {
			content = &ed.Contents[i]
		}
	}
	badge := ed.Badges[current.ID]
	if content == nil || (badge != 1 && badge != 2) {
		// Vigente sem conteúdo ou sem aprovação conhecida: na dúvida, segue
		// "em análise", só com dados oficiais.
		return
	}
	out.UnderReview = false
	out.Summary, out.ShortSummary, out.ReviewBadge = content.Summary, content.ShortSummary, badge
	for _, id := range content.TopicIDs {
		if t, ok := ed.Topics[id]; ok {
			out.Topics = append(out.Topics, t.Name)
		}
	}
	if current.OfficialTextURL != "" {
		out.OfficialTextURL = current.OfficialTextURL
	}
	if pending != nil {
		out.PendingVersion = &PublicPendingVersion{Label: pending.Label, OfficialTextURL: pending.OfficialTextURL}
	}
	// Só as da vigente: quando uma Versão nova vira vigente, as Erratas da
	// anterior saem junto com o conteúdo dela.
	for _, e := range ed.Errata {
		if e.VersionID == current.ID {
			out.Errata = append(out.Errata, e)
		}
	}
	sort.SliceStable(out.Errata, func(i, j int) bool { return out.Errata[i].ApprovedAt.After(out.Errata[j].ApprovedAt) })
	out.AnnotatedText = annotatedText(current.ID, ed)
}

// annotatedText monta o Documento comentado da Versão vigente. O texto
// fonte da Estruturação (source_text) e os avisos nunca saem.
func annotatedText(versionID string, ed Editorial) []PublicProvision {
	confirmed := false
	for _, s := range ed.Structurings {
		if s.VersionID == versionID && s.State == StructuringConfirmed {
			confirmed = true
		}
	}
	ps := ed.Provisions[versionID]
	if !confirmed || len(ps) == 0 {
		return nil
	}
	byAddr := map[string][]string{}
	for _, c := range ed.Comments {
		if c.VersionID == versionID && c.DeletedAt == nil && !c.NeedsConfirmation {
			byAddr[c.ProvisionAddress] = append(byAddr[c.ProvisionAddress], c.Body)
		}
	}
	sorted := append([]Provision(nil), ps...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })
	out := make([]PublicProvision, 0, len(sorted))
	for _, p := range sorted {
		cs := byAddr[p.Address]
		if cs == nil || !p.Kind.Commentable() {
			cs = []string{}
		}
		out = append(out, PublicProvision{
			Address: p.Address, Label: p.Label, Kind: string(p.Kind), Text: p.Text, Depth: p.Depth, Comments: cs,
		})
	}
	return out
}
