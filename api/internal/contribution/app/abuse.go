package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

var (
	// ErrRateLimited: o IP passou do limite da hora.
	ErrRateLimited = errors.New("contribution: limite de tentativas por hora")
	// ErrSubmissionsPaused: a Fila chegou ao teto de pendentes e o
	// formulário pausou.
	ErrSubmissionsPaused = errors.New("contribution: envio pausado (teto de pendentes)")
)

// RateCounter guarda o rate limit no Postgres, para valer com mais de uma
// instância da API. Nunca vê o IP: só o HMAC dele.
type RateCounter interface {
	// DailyKey devolve a chave do dia; se o dia ainda não tem uma, grava
	// candidate e a devolve.
	DailyKey(ctx context.Context, day time.Time, candidate []byte) ([]byte, error)
	// Hit soma 1 ao contador e devolve o total na janela.
	Hit(ctx context.Context, subject []byte, bucket domain.RateBucket, window time.Time) (int, error)
	// Purge apaga as chaves de dias anteriores a keysBefore e os contadores
	// de janelas anteriores a hitsBefore.
	Purge(ctx context.Context, keysBefore, hitsBefore time.Time) error
}

// RateLimiter conta as tentativas por IP numa janela de uma hora.
type RateLimiter struct {
	counter RateCounter
	clock   clock.Clock
	random  io.Reader
}

func NewRateLimiter(c RateCounter, clk clock.Clock, random io.Reader) *RateLimiter {
	return &RateLimiter{counter: c, clock: clk, random: random}
}

// Allow conta uma tentativa de clientKey (o IP, ou o /64 no IPv6) e
// devolve ErrRateLimited se ela passa de perHour. Toda tentativa conta,
// válida ou não: o limite também impede sondar o formulário.
func (l *RateLimiter) Allow(ctx context.Context, bucket domain.RateBucket, clientKey string, perHour int) error {
	now := l.clock.Now()
	candidate := make([]byte, domain.RateKeySize)
	if _, err := io.ReadFull(l.random, candidate); err != nil {
		return fmt.Errorf("sorteando chave do dia: %w", err)
	}
	key, err := l.counter.DailyKey(ctx, domain.RateDay(now), candidate)
	if err != nil {
		return err
	}
	n, err := l.counter.Hit(ctx, domain.RateSubject(key, clientKey), bucket, domain.RateWindow(now))
	if err != nil {
		return err
	}
	if n > perHour {
		return ErrRateLimited
	}
	return nil
}

// PurgeRateLimits apaga a chave do dia anterior e os contadores de horas
// passadas. Roda de hora em hora no worker: a chave de um dia some, no
// máximo, uma hora depois de o dia acabar.
type PurgeRateLimits struct {
	counter RateCounter
	clock   clock.Clock
}

func NewPurgeRateLimits(c RateCounter, clk clock.Clock) *PurgeRateLimits {
	return &PurgeRateLimits{counter: c, clock: clk}
}

func (uc *PurgeRateLimits) Execute(ctx context.Context) error {
	now := uc.clock.Now()
	return uc.counter.Purge(ctx, domain.RateDay(now), domain.RateWindow(now))
}

// Availability diz se o formulário está aberto: fecha quando a Fila chega
// ao teto de pendentes.
type Availability struct {
	contributions ContributionRepo
	pendingCap    int
}

func NewAvailability(c ContributionRepo, pendingCap int) *Availability {
	return &Availability{contributions: c, pendingCap: pendingCap}
}

// Execute devolve ErrSubmissionsPaused se o teto foi atingido.
func (uc *Availability) Execute(ctx context.Context) error {
	n, err := uc.contributions.CountPending(ctx)
	if err != nil {
		return err
	}
	if n >= uc.pendingCap {
		return ErrSubmissionsPaused
	}
	return nil
}
