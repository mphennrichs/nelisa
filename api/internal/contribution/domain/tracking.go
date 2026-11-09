package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// TrackingCodeLen é o tamanho do código de acompanhamento sem hífens: 12
// caracteres de base32 Crockford, 60 bits.
const TrackingCodeLen = 12

// crockford é o alfabeto base32 de Crockford: sem I, L, O e U.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ErrInvalidTrackingCode: o texto não tem a forma de um código.
var ErrInvalidTrackingCode = errors.New("contribution: código de acompanhamento inválido")

// TrackingCode é o código de acompanhamento já canônico (12 caracteres,
// maiúsculas, sem hífen). Só o Leitor o conhece: o banco guarda o Hash.
type TrackingCode string

// NewTrackingCode sorteia um código com rnd (crypto/rand em produção).
func NewTrackingCode(rnd io.Reader) (TrackingCode, error) {
	var b [TrackingCodeLen]byte
	if _, err := io.ReadFull(rnd, b[:]); err != nil {
		return "", fmt.Errorf("sorteando código: %w", err)
	}
	out := make([]byte, TrackingCodeLen)
	for i, v := range b {
		// 256 é múltiplo de 32: v%32 não tem viés.
		out[i] = crockford[v%32]
	}
	return TrackingCode(out), nil
}

// ParseTrackingCode lê o que o Leitor digitou: aceita minúsculas, hífens e
// espaços, e lê O como 0 e I/L como 1 (regra de Crockford).
func ParseTrackingCode(raw string) (TrackingCode, error) {
	if len(raw) > 64 {
		return "", ErrInvalidTrackingCode
	}
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		switch {
		case r == '-' || r == ' ':
			continue
		case r == 'O':
			r = '0'
		case r == 'I' || r == 'L':
			r = '1'
		}
		if !strings.ContainsRune(crockford, r) {
			return "", ErrInvalidTrackingCode
		}
		b.WriteRune(r)
	}
	if b.Len() != TrackingCodeLen {
		return "", ErrInvalidTrackingCode
	}
	return TrackingCode(b.String()), nil
}

// Display é o código como o Leitor vê: K7Q4-9MZD-2HXP.
func (c TrackingCode) Display() string {
	s := string(c)
	if len(s) != TrackingCodeLen {
		return s
	}
	return s[0:4] + "-" + s[4:8] + "-" + s[8:12]
}

// Hash é o que o banco guarda: sha256 em hex. 60 bits sorteados tornam
// inútil adivinhar pelo hash; sem sal, a consulta é um índice só.
func (c TrackingCode) Hash() string {
	sum := sha256.Sum256([]byte(c))
	return hex.EncodeToString(sum[:])
}
