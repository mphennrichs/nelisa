package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Ciclo editorial (CONTEXT.md, "Versão", "Resumo", "Tema"): a Versão do
// Documento integral, o conteúdo que o Nelisa produz sobre ela e os Temas.

var (
	ErrVersionNotFound = errors.New("proposition: versão não encontrada")
	// ErrPendingVersionExists: já há uma Versão em análise; uma nova só
	// depois que ela for aprovada.
	ErrPendingVersionExists = errors.New("proposition: já existe uma versão em análise")
	// ErrVersionNotEditable: só a Versão em análise tem conteúdo editável.
	// O da vigente só muda por Correção menor ou Errata.
	ErrVersionNotEditable = errors.New("proposition: só a versão em análise pode ser editada")
	ErrInvalidLabel       = errors.New("proposition: rótulo da versão inválido (obrigatório, até 120 caracteres)")
	ErrInvalidVersionURL  = errors.New("proposition: link do documento integral inválido (http ou https)")
	// ErrStaleContent: alguém gravou o conteúdo depois que ele foi lido.
	ErrStaleContent        = errors.New("proposition: o conteúdo mudou desde que foi aberto")
	ErrSummaryTooLong      = errors.New("proposition: resumo longo demais (até 5.000 caracteres)")
	ErrShortSummaryTooLong = errors.New("proposition: resumo curto longo demais (até 280 caracteres)")
	ErrTooManyTopics       = errors.New("proposition: no máximo 3 temas")
	ErrDuplicateTopic      = errors.New("proposition: tema repetido")
	ErrNothingToConfirm    = errors.New("proposition: não há conteúdo trazido a confirmar")
	ErrTopicNotFound       = errors.New("proposition: tema não encontrado")
	ErrTopicInactive       = errors.New("proposition: tema desativado")
	ErrInvalidTopicName    = errors.New("proposition: nome de tema inválido (obrigatório, até 60 caracteres)")
	ErrTopicNameTaken      = errors.New("proposition: já existe um tema com esse nome")
	ErrVersionNotPending   = errors.New("proposition: a versão não está em análise")
)

// Limites do conteúdo editorial. Nunca cortamos: texto acima do limite é
// recusado, porque um Resumo curto cortado pode mudar o sentido.
const (
	MaxSummaryLen      = 5_000
	MaxShortSummaryLen = 280
	MaxTopics          = 3
	MaxLabelLen        = 120
	MaxTopicNameLen    = 60
)

// VersionState: em análise → vigente → substituída.
type VersionState string

const (
	VersionPending    VersionState = "pending"
	VersionCurrent    VersionState = "current"
	VersionSuperseded VersionState = "superseded"
)

// ImportedVersionLabel é o rótulo da Versão 1 criada pela Importação.
const ImportedVersionLabel = "Texto original"

// Version é uma edição do Documento integral.
type Version struct {
	ID              string
	PropositionID   string
	Seq             int
	Label           string
	OfficialTextURL string
	State           VersionState
	// CreatedBy vazio quando veio da Importação.
	CreatedBy string
	CreatedAt time.Time
}

// NewVersion valida rótulo e link de uma Versão criada à mão pelo Editor.
func NewVersion(id, propositionID string, seq int, label, url, createdBy string, now time.Time) (Version, error) {
	label = strings.TrimSpace(label)
	if label == "" || utf8.RuneCountInString(label) > MaxLabelLen {
		return Version{}, ErrInvalidLabel
	}
	url = strings.TrimSpace(url)
	if url == "" || len(url) > maxURLLen || !IsHTTPURL(url) {
		return Version{}, ErrInvalidVersionURL
	}
	return Version{
		ID: id, PropositionID: propositionID, Seq: seq, Label: label, OfficialTextURL: url,
		State: VersionPending, CreatedBy: createdBy, CreatedAt: now,
	}, nil
}

// NewImportedVersion é a Versão 1, em análise, com o link que a fonte deu
// (vazio se a fonte não deu um link válido).
func NewImportedVersion(id string, p Proposition, now time.Time) Version {
	return Version{
		ID: id, PropositionID: p.ID, Seq: 1, Label: ImportedVersionLabel,
		OfficialTextURL: p.OfficialTextURL, State: VersionPending, CreatedAt: now,
	}
}

// Content é o conteúdo editorial de uma Versão: Resumo, Resumo curto e
// Temas, mais o que a equipe precisa para a revisão (Autoria editorial,
// trava otimista). Nada além de Resumo, Resumo curto e Temas da Versão
// vigente sai para o Leitor.
type Content struct {
	VersionID    string
	Summary      string
	ShortSummary string
	TopicIDs     []string
	// NeedsConfirmation: veio copiado da Versão anterior e ainda não foi
	// confirmado nem reescrito.
	NeedsConfirmation bool
	// Authors: todo mundo que já gravou ou confirmou este conteúdo.
	Authors   []string
	Revision  int
	UpdatedBy string
	UpdatedAt time.Time
}

// NewEmptyContent é o conteúdo de uma Versão que não herdou nada.
func NewEmptyContent(versionID string, now time.Time) Content {
	return Content{VersionID: versionID, TopicIDs: []string{}, Authors: []string{}, UpdatedAt: now}
}

// CopyForNewVersion leva o conteúdo da vigente para a Versão nova, marcado
// a confirmar. A Autoria vai junto: quem escreveu o texto herdado continua
// sem poder aprová-lo (quatro olhos), mesmo sem ter mexido na Versão nova.
func (c Content) CopyForNewVersion(versionID string, now time.Time) Content {
	return Content{
		VersionID: versionID, Summary: c.Summary, ShortSummary: c.ShortSummary,
		TopicIDs: slices.Clone(nonNilStrings(c.TopicIDs)), NeedsConfirmation: true,
		Authors: slices.Clone(nonNilStrings(c.Authors)), UpdatedAt: now,
	}
}

// ContentEdit é o que o Editor manda gravar.
type ContentEdit struct {
	Summary      string
	ShortSummary string
	TopicIDs     []string
}

// Normalize limpa e valida a edição: espaços nas pontas, quebras de linha
// "\r\n" viram "\n", limites sem corte, até 3 Temas sem repetição.
func (e ContentEdit) Normalize() (ContentEdit, error) {
	out := ContentEdit{
		Summary:      strings.TrimSpace(strings.ReplaceAll(e.Summary, "\r\n", "\n")),
		ShortSummary: strings.TrimSpace(strings.ReplaceAll(e.ShortSummary, "\r\n", "\n")),
		TopicIDs:     []string{},
	}
	if utf8.RuneCountInString(out.Summary) > MaxSummaryLen {
		return ContentEdit{}, ErrSummaryTooLong
	}
	if utf8.RuneCountInString(out.ShortSummary) > MaxShortSummaryLen {
		return ContentEdit{}, ErrShortSummaryTooLong
	}
	if len(e.TopicIDs) > MaxTopics {
		return ContentEdit{}, ErrTooManyTopics
	}
	for _, id := range e.TopicIDs {
		id = strings.TrimSpace(id)
		if slices.Contains(out.TopicIDs, id) {
			return ContentEdit{}, ErrDuplicateTopic
		}
		out.TopicIDs = append(out.TopicIDs, id)
	}
	return out, nil
}

// Apply grava a edição de editorID. Devolve os campos que mudaram (para a
// Auditoria, que nunca leva o texto). Sem mudança, nada acontece: abrir e
// salvar sem mexer não torna ninguém autor. Salvar não confirma o
// conteúdo trazido; isso é uma ação à parte (Confirm).
func (c *Content) Apply(e ContentEdit, editorID string, now time.Time) []string {
	var changed []string
	if c.Summary != e.Summary {
		changed = append(changed, "summary")
	}
	if c.ShortSummary != e.ShortSummary {
		changed = append(changed, "short_summary")
	}
	if !slices.Equal(nonNilStrings(c.TopicIDs), nonNilStrings(e.TopicIDs)) {
		changed = append(changed, "topic_ids")
	}
	if len(changed) == 0 {
		return nil
	}
	c.Summary, c.ShortSummary, c.TopicIDs = e.Summary, e.ShortSummary, slices.Clone(e.TopicIDs)
	c.touch(editorID, now)
	return changed
}

// Confirm é a ação explícita "confirmar conteúdo trazido". Quem confirma
// assume o texto como seu: entra na Autoria.
func (c *Content) Confirm(editorID string, now time.Time) error {
	if !c.NeedsConfirmation {
		return ErrNothingToConfirm
	}
	c.NeedsConfirmation = false
	c.touch(editorID, now)
	return nil
}

func (c *Content) touch(editorID string, now time.Time) {
	if !slices.Contains(c.Authors, editorID) {
		c.Authors = append(slices.Clone(c.Authors), editorID)
	}
	c.Revision++
	c.UpdatedBy, c.UpdatedAt = editorID, now
}

// HasAuthor diz se userID já escreveu este conteúdo (quatro olhos).
func (c Content) HasAuthor(userID string) bool { return slices.Contains(c.Authors, userID) }

// Missing é o que falta no conteúdo para a Versão poder ser aprovada.
type Missing struct {
	Code    string
	Message string
}

// Códigos do que falta para aprovar (estáveis: vão para a API).
const (
	MissingSummary           = "summary"
	MissingShortSummary      = "short_summary"
	MissingTopics            = "topics"
	MissingInactiveTopics    = "inactive_topics"
	MissingNeedsConfirmation = "needs_confirmation"
)

// MissingForApproval lista o que impede a aprovação: Resumo e Resumo curto
// não vazios, 1 a 3 Temas todos ativos, conteúdo trazido confirmado.
// topics são os Temas conhecidos, por id.
func (c Content) MissingForApproval(topics map[string]Topic) []Missing {
	var out []Missing
	if strings.TrimSpace(c.Summary) == "" {
		out = append(out, Missing{MissingSummary, "Escreva o Resumo."})
	}
	if strings.TrimSpace(c.ShortSummary) == "" {
		out = append(out, Missing{MissingShortSummary, "Escreva o Resumo curto."})
	}
	active, inactive := 0, 0
	for _, id := range c.TopicIDs {
		if t, ok := topics[id]; ok && t.Active {
			active++
		} else {
			inactive++
		}
	}
	if active == 0 || active > MaxTopics {
		out = append(out, Missing{MissingTopics, "Escolha de 1 a 3 Temas."})
	}
	if inactive > 0 {
		out = append(out, Missing{MissingInactiveTopics, "Troque os Temas desativados."})
	}
	if c.NeedsConfirmation {
		out = append(out, Missing{MissingNeedsConfirmation, "Confirme ou reescreva o conteúdo trazido da Versão anterior."})
	}
	return out
}

// Topic é um Tema da lista fechada mantida pelo Admin.
type Topic struct {
	ID        string
	Name      string
	Active    bool
	CreatedAt time.Time
}

// NormalizeTopicName limpa e valida o nome de um Tema.
func NormalizeTopicName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || utf8.RuneCountInString(name) > MaxTopicNameLen {
		return "", ErrInvalidTopicName
	}
	return name, nil
}

// NewTopic cria um Tema ativo.
func NewTopic(id, name string, now time.Time) (Topic, error) {
	n, err := NormalizeTopicName(name)
	if err != nil {
		return Topic{}, err
	}
	return Topic{ID: id, Name: n, Active: true, CreatedAt: now}, nil
}

// TopicsByID indexa os Temas pelo id.
func TopicsByID(ts []Topic) map[string]Topic {
	out := make(map[string]Topic, len(ts))
	for _, t := range ts {
		out[t.ID] = t
	}
	return out
}

// CheckTopicChoice confere os Temas escolhidos: todos existem e os novos
// estão ativos. Um Tema desativado depois de escolhido pode continuar
// (some da escolha, não das Versões que já o usam).
func CheckTopicChoice(chosen, previous []string, topics map[string]Topic) error {
	for _, id := range chosen {
		t, ok := topics[id]
		if !ok {
			return ErrTopicNotFound
		}
		if !t.Active && !slices.Contains(previous, id) {
			return ErrTopicInactive
		}
	}
	return nil
}

// ReturnToReview: uma Versão nova devolve a Proposição pronta para "em
// revisão" (CONTEXT.md, "Versão"). Nos outros estados nada muda.
func (p *Proposition) ReturnToReview(now time.Time) {
	if p.ReviewState == ReviewReady {
		p.ReviewState = ReviewInReview
		p.UpdatedAt = now
	}
}

// MarkReady é a aprovação: "em revisão" → "pronto". Só a partir de "em
// revisão"; rascunho precisa ser enviado para revisão antes.
func (p *Proposition) MarkReady(now time.Time) error {
	if p.ReviewState != ReviewInReview {
		return ErrInvalidTransition
	}
	p.ReviewState = ReviewReady
	p.UpdatedAt = now
	return nil
}

// FindVersions separa a vigente e a em análise de uma lista de Versões.
func FindVersions(vs []Version) (current, pending *Version) {
	for i := range vs {
		switch vs[i].State {
		case VersionCurrent:
			current = &vs[i]
		case VersionPending:
			pending = &vs[i]
		}
	}
	return current, pending
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
