package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"time"
)

// RateBucket é o que um limite conta: o envio ou a consulta.
type RateBucket string

const (
	BucketSubmit RateBucket = "submit"
	BucketLookup RateBucket = "lookup"
)

// RateKeySize é o tamanho da chave do dia, em bytes.
const RateKeySize = 32

// RateSubject é o que o rate limit grava no lugar do IP: um HMAC-SHA256
// com a chave do dia. Sem a chave, que some no dia seguinte, não há como
// voltar ao IP nem testar um IP contra o contador.
func RateSubject(dayKey []byte, clientKey string) []byte {
	m := hmac.New(sha256.New, dayKey)
	m.Write([]byte(clientKey))
	return m.Sum(nil)
}

// RateDay é o início do dia (UTC) da chave que vale em now.
func RateDay(now time.Time) time.Time { return now.UTC().Truncate(24 * time.Hour) }

// RateWindow é o início da hora (UTC) do contador que vale em now.
func RateWindow(now time.Time) time.Time { return now.UTC().Truncate(time.Hour) }
