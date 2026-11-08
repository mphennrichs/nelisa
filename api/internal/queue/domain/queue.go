// Package domain tem as regras da Fila de revisão e da Posse (CONTEXT.md):
// quem pode editar uma Proposição, quando a Posse expira e em que ordem a
// equipe vê o trabalho. Só Go padrão.
package domain

import (
	"errors"
	"sort"
	"time"
)

var (
	// ErrClaimTaken: outra pessoa tem a Posse e ela vale. Use errors.As
	// com *ClaimTakenError para saber quem.
	ErrClaimTaken = errors.New("queue: outra pessoa está com a posse")
	// ErrNotHolder: só quem tem a Posse, ou Revisor/Admin, a solta.
	ErrNotHolder     = errors.New("queue: só quem tem a posse, ou revisor/admin, pode soltá-la")
	ErrInvalidSphere = errors.New("queue: esfera inválida")
)

// ClaimTakenError diz quem tem a Posse (nome visível só para a equipe).
type ClaimTakenError struct {
	HolderID   string
	HolderName string
}

func (e *ClaimTakenError) Error() string {
	return "queue: " + e.HolderName + " está com a posse"
}

func (e *ClaimTakenError) Is(target error) bool { return target == ErrClaimTaken }

// ClaimTTL: a Posse expira com 3 dias sem atividade.
const ClaimTTL = 72 * time.Hour

// Claim é a Posse de uma Proposição.
type Claim struct {
	PropositionID  string
	UserID         string
	ClaimedAt      time.Time
	LastActivityAt time.Time
}

// ExpiresAt é quando a Posse deixa de valer sem nova atividade.
func (c Claim) ExpiresAt() time.Time { return c.LastActivityAt.Add(ClaimTTL) }

// Valid diz se a Posse vale agora: não expirou e a conta de quem a tem
// está ativa. Conta desativada perde as posses sem precisar de ninguém
// soltá-las: quem checa ignora (decisão do desenho da fase 2).
func (c *Claim) Valid(now time.Time, holderActive bool) bool {
	return c != nil && holderActive && now.Before(c.ExpiresAt())
}

// Take é userID assumindo a Posse. existing é a linha guardada (nil se não
// há) e valid diz se ela vale (ver Valid). Se vale e é de outra pessoa,
// ErrClaimTaken; se já é de userID, só renova a atividade.
func Take(existing *Claim, valid bool, propositionID, userID string, now time.Time) (Claim, error) {
	if valid && existing.UserID != userID {
		return Claim{}, &ClaimTakenError{HolderID: existing.UserID}
	}
	if valid {
		c := *existing
		c.LastActivityAt = now
		return c, nil
	}
	return Claim{PropositionID: propositionID, UserID: userID, ClaimedAt: now, LastActivityAt: now}, nil
}

// Sphere é a Esfera, que manda na ordem da Fila.
type Sphere string

const (
	SphereFederal   Sphere = "federal"
	SphereState     Sphere = "state"
	SphereMunicipal Sphere = "municipal"
)

// ParseSphere aceita as três Esferas.
func ParseSphere(s string) (Sphere, error) {
	switch Sphere(s) {
	case SphereFederal, SphereState, SphereMunicipal:
		return Sphere(s), nil
	}
	return "", ErrInvalidSphere
}

// rank: federal > estadual > municipal. Esfera desconhecida vai por último.
func (s Sphere) rank() int {
	switch s {
	case SphereFederal:
		return 0
	case SphereState:
		return 1
	case SphereMunicipal:
		return 2
	}
	return 3
}

// Reason é por que a Proposição está na Fila.
type Reason string

const (
	ReasonReview       Reason = "review"
	ReasonSecondReview Reason = "second_review"
)

// Item é uma entrada da Fila.
type Item struct {
	PropositionID string
	Sphere        Sphere
	Reason        Reason
	// VoteAt é a votação marcada mais próxima. Ainda não há esse dado: todo
	// item cai em "sem votação" (nil), mas a ordem já o respeita.
	VoteAt *time.Time
	// Since é desde quando o item está na Fila (a Versão em análise existe,
	// ou a aprovação sem quatro olhos aconteceu).
	Since time.Time
}

// Sort ordena a Fila: sempre por Esfera; dentro dela, votação mais
// próxima primeiro, depois os sem votação, do mais antigo para o mais
// novo.
func Sort[T any](items []T, key func(T) Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := key(items[i]), key(items[j])
		if ra, rb := a.Sphere.rank(), b.Sphere.rank(); ra != rb {
			return ra < rb
		}
		switch {
		case a.VoteAt != nil && b.VoteAt != nil && !a.VoteAt.Equal(*b.VoteAt):
			return a.VoteAt.Before(*b.VoteAt)
		case a.VoteAt != nil && b.VoteAt == nil:
			return true
		case a.VoteAt == nil && b.VoteAt != nil:
			return false
		}
		if !a.Since.Equal(b.Since) {
			return a.Since.Before(b.Since)
		}
		if a.PropositionID != b.PropositionID {
			return a.PropositionID < b.PropositionID
		}
		return a.Reason < b.Reason
	})
}

// CanRelease diz quem solta uma Posse válida: quem a tem, ou Revisor e
// Admin (de qualquer pessoa).
func CanRelease(holderID, actorID string, actorIsReviewer bool) bool {
	return holderID == actorID || actorIsReviewer
}
