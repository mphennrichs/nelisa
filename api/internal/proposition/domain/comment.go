package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// MaxCommentLen limita o texto de um Comentário (texto simples, com
// parágrafos).
const MaxCommentLen = 3000

var (
	ErrCommentNotFound = errors.New("comentário não encontrado")
	// ErrEmptyComment: Comentário sem texto.
	ErrEmptyComment = errors.New("proposition: escreva o Comentário")
	// ErrCommentTooLong: mais que MaxCommentLen caracteres.
	ErrCommentTooLong = errors.New("proposition: Comentário longo demais")
	// ErrStructuringNotConfirmed: só se comenta depois de confirmar os
	// Dispositivos (senão a árvore ainda pode mudar debaixo do Comentário).
	ErrStructuringNotConfirmed = errors.New("proposition: confirme a Estruturação antes de comentar")
	// ErrProvisionNotFound: o endereço não é de um Dispositivo desta Versão.
	ErrProvisionNotFound = errors.New("proposition: Dispositivo não encontrado")
	// ErrProvisionNotCommentable: agrupamento (Capítulo, Seção) é só título.
	ErrProvisionNotCommentable = errors.New("proposition: este Dispositivo não recebe Comentário")
)

// AnchorsRemovedError: o texto novo da Estruturação some com Dispositivos
// que têm Comentário. O Editor move ou apaga esses Comentários antes; a
// âncora nunca fica órfã.
type AnchorsRemovedError struct {
	Addresses []string
}

func (e *AnchorsRemovedError) Error() string {
	return "proposition: o texto novo apaga Dispositivos comentados: " + strings.Join(e.Addresses, ", ")
}

// Comment é um Comentário: análise pública de um Editor ou Revisor presa a
// um Dispositivo de uma Versão. Só aparece para o Leitor quando a Versão
// é a vigente.
type Comment struct {
	ID               string
	VersionID        string
	ProvisionAddress string
	Body             string
	// NeedsConfirmation: trazido da Versão anterior (Dispositivo com o
	// mesmo endereço e o mesmo texto); precisa ser confirmado ou reescrito.
	NeedsConfirmation bool
	// CopiedFrom é o Comentário da Versão anterior de onde veio ("" se
	// foi escrito nesta Versão).
	CopiedFrom string
	// Authors: todo mundo que já escreveu ou confirmou este Comentário.
	Authors   []string
	Revision  int
	CreatedBy string
	CreatedAt time.Time
	UpdatedBy string
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// NormalizeCommentBody limpa e valida o texto de um Comentário.
func NormalizeCommentBody(body string) (string, error) {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	switch {
	case body == "":
		return "", ErrEmptyComment
	case len([]rune(body)) > MaxCommentLen:
		return "", ErrCommentTooLong
	}
	return body, nil
}

// CheckCommentable diz se dá para comentar o Dispositivo address desta
// Estruturação.
func CheckCommentable(s Structuring, provisions []Provision, address string) error {
	if s.State != StructuringConfirmed {
		return ErrStructuringNotConfirmed
	}
	for _, p := range provisions {
		if p.Address == address {
			if !p.Kind.Commentable() {
				return ErrProvisionNotCommentable
			}
			return nil
		}
	}
	return ErrProvisionNotFound
}

// NewComment cria um Comentário escrito agora.
func NewComment(id, versionID, address, body, editorID string, now time.Time) Comment {
	return Comment{
		ID: id, VersionID: versionID, ProvisionAddress: address, Body: body,
		Authors: []string{editorID}, CreatedBy: editorID, CreatedAt: now, UpdatedBy: editorID, UpdatedAt: now,
	}
}

// Edit troca o texto; quem edita entra na Autoria. Editar um Comentário
// trazido não o confirma (como o Resumo).
func (c *Comment) Edit(body, editorID string, now time.Time) bool {
	if body == c.Body {
		return false
	}
	c.Body = body
	c.touch(editorID, now)
	return true
}

// Confirm é a ação explícita "confirmar comentário trazido".
func (c *Comment) Confirm(editorID string, now time.Time) error {
	if !c.NeedsConfirmation {
		return ErrNothingToConfirm
	}
	c.NeedsConfirmation = false
	c.touch(editorID, now)
	return nil
}

// Delete apaga (soft): some da equipe e do público, fica no banco.
func (c *Comment) Delete(editorID string, now time.Time) {
	t := now
	c.DeletedAt = &t
	c.touch(editorID, now)
}

func (c *Comment) touch(editorID string, now time.Time) {
	if !slices.Contains(c.Authors, editorID) {
		c.Authors = append(slices.Clone(c.Authors), editorID)
	}
	c.Revision++
	c.UpdatedBy, c.UpdatedAt = editorID, now
}

// RemovedAnchors lista os endereços com Comentário que não existem mais
// entre os Dispositivos novos.
func RemovedAnchors(comments []Comment, provisions []Provision) []string {
	exists := make(map[string]bool, len(provisions))
	for _, p := range provisions {
		exists[p.Address] = true
	}
	var out []string
	for _, c := range comments {
		if c.DeletedAt == nil && !exists[c.ProvisionAddress] && !slices.Contains(out, c.ProvisionAddress) {
			out = append(out, c.ProvisionAddress)
		}
	}
	return out
}

// CarryComments escolhe os Comentários da Versão vigente que vão para a
// Versão nova como "a confirmar": só os de Dispositivos com o mesmo
// endereço e o mesmo texto na Versão nova (CONTEXT.md, "Versão vigente /
// em análise"). already são os ids da vigente já trazidos antes (confirmar
// de novo depois de reprocessar não duplica).
func CarryComments(from []Comment, fromProvisions, toProvisions []Provision, already map[string]bool) []Comment {
	hashOf := func(ps []Provision) map[string]string {
		m := make(map[string]string, len(ps))
		for _, p := range ps {
			m[p.Address] = p.TextHash
		}
		return m
	}
	oldHash, newHash := hashOf(fromProvisions), hashOf(toProvisions)
	var out []Comment
	for _, c := range from {
		if c.DeletedAt != nil || c.NeedsConfirmation || already[c.ID] {
			continue
		}
		h, ok := newHash[c.ProvisionAddress]
		if !ok || h != oldHash[c.ProvisionAddress] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// MissingCommentConfirmation: Comentário trazido e não confirmado impede a
// aprovação.
func MissingCommentConfirmation(comments []Comment) []Missing {
	for _, c := range comments {
		if c.DeletedAt == nil && c.NeedsConfirmation {
			return []Missing{{MissingCommentsUnconfirmed, "Confirme, reescreva ou apague os Comentários trazidos da Versão anterior."}}
		}
	}
	return nil
}

// CopyForNewVersion traz o Comentário para a Versão nova, a confirmar,
// com a mesma Autoria.
func (c Comment) CopyForNewVersion(id, versionID string, now time.Time) Comment {
	return Comment{
		ID: id, VersionID: versionID, ProvisionAddress: c.ProvisionAddress, Body: c.Body,
		NeedsConfirmation: true, CopiedFrom: c.ID, Authors: slices.Clone(c.Authors),
		CreatedBy: c.CreatedBy, CreatedAt: now, UpdatedAt: now,
	}
}

// LiveComments tira os apagados.
func LiveComments(cs []Comment) []Comment {
	out := make([]Comment, 0, len(cs))
	for _, c := range cs {
		if c.DeletedAt == nil {
			out = append(out, c)
		}
	}
	return out
}
