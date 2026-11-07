// Package clock é a implementação real da porta Clock que os módulos usam,
// para que os testes possam fixar o tempo.
package clock

import "time"

// Clock é a porta: os casos de uso dependem dela, nunca de time.Now.
type Clock interface {
	Now() time.Time
}

// System devolve a hora real em UTC.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fixed é um relógio parado, para testes.
type Fixed struct{ T time.Time }

func (f *Fixed) Now() time.Time { return f.T }

// Advance anda o relógio de teste.
func (f *Fixed) Advance(d time.Duration) { f.T = f.T.Add(d) }
