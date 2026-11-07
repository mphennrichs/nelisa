// Package fakes tem implementações em memória das portas de team/app,
// escritas à mão (sem framework de mock), usadas pelos testes de app/ e de
// httpapi/. A transação falsa guarda uma cópia do estado e a restaura se a
// função devolver erro, para os testes provarem que mudança e Auditoria
// são desfeitas juntas.
package fakes

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/team/app"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

// RecordedEvent é um evento de Auditoria e se ele chegou dentro de uma
// transação.
type RecordedEvent struct {
	app.AuditEvent
	InTx bool
}

// World é o "banco" em memória.
type World struct {
	mu     sync.Mutex
	users  map[string]domain.User
	tokens map[string]domain.RefreshToken
	events []RecordedEvent
	inTx   bool

	// AuditErr, se definido, faz toda gravação de Auditoria falhar.
	AuditErr error
	// LockCalls conta as chamadas a LockUsers.
	LockCalls int
}

func NewWorld() *World {
	return &World{users: map[string]domain.User{}, tokens: map[string]domain.RefreshToken{}}
}

func (w *World) Users() app.UserRepo          { return users{w} }
func (w *World) Tokens() app.RefreshTokenRepo { return tokens{w} }
func (w *World) Audit() app.AuditRecorder     { return audit{w} }
func (w *World) Tx() app.TxRunner             { return tx{w} }

// Events devolve uma cópia dos eventos gravados.
func (w *World) Events() []RecordedEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.events)
}

// TokensOf devolve as sessões de um usuário.
func (w *World) TokensOf(userID string) []domain.RefreshToken {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []domain.RefreshToken
	for _, t := range w.tokens {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	return out
}

// Put grava um usuário direto, sem regra nem Auditoria (montagem de teste).
func (w *World) Put(u domain.User) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.users[u.ID] = u
}

// --- transação ---

type tx struct{ w *World }

func (t tx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	w := t.w
	w.mu.Lock()
	if w.inTx {
		w.mu.Unlock()
		return fn(ctx)
	}
	users, tokens, events := maps.Clone(w.users), maps.Clone(w.tokens), slices.Clone(w.events)
	w.inTx = true
	w.mu.Unlock()

	err := fn(ctx)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.inTx = false
	if err != nil {
		w.users, w.tokens, w.events = users, tokens, events
	}
	return err
}

// --- auditoria ---

type audit struct{ w *World }

func (a audit) Record(_ context.Context, e app.AuditEvent) error {
	a.w.mu.Lock()
	defer a.w.mu.Unlock()
	if a.w.AuditErr != nil {
		return a.w.AuditErr
	}
	a.w.events = append(a.w.events, RecordedEvent{AuditEvent: e, InTx: a.w.inTx})
	return nil
}

// --- usuários ---

type users struct{ w *World }

func (r users) LockUsers(context.Context) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.LockCalls++
	return nil
}

func (r users) Count(context.Context) (int, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	return len(r.w.users), nil
}

func (r users) CountAdmins(context.Context) (int, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	n := 0
	for _, u := range r.w.users {
		if u.Role == domain.RoleAdmin {
			n++
		}
	}
	return n, nil
}

func (r users) Create(_ context.Context, u domain.User) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, o := range r.w.users {
		if strings.EqualFold(o.Email, u.Email) {
			return domain.ErrEmailTaken
		}
	}
	r.w.users[u.ID] = u
	return nil
}

func (r users) FindByID(_ context.Context, id string) (domain.User, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	u, ok := r.w.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (r users) FindByEmail(_ context.Context, email string) (domain.User, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, u := range r.w.users {
		if strings.EqualFold(u.Email, strings.TrimSpace(email)) {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (r users) List(context.Context) ([]domain.User, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	out := slices.Collect(maps.Values(r.w.users))
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r users) UpdatePassword(_ context.Context, u domain.User, prevHash string) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.users[u.ID]
	if !ok {
		return domain.ErrUserNotFound
	}
	if cur.PasswordHash != prevHash {
		return domain.ErrCredentialsChanged
	}
	cur.PasswordHash, cur.MustChangePassword, cur.SessionsValidAfter = u.PasswordHash, u.MustChangePassword, u.SessionsValidAfter
	r.w.users[u.ID] = cur
	return nil
}

func (r users) UpdateRole(_ context.Context, id string, role domain.Role) (domain.User, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	u, ok := r.w.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	u.Role = role
	r.w.users[id] = u
	return u, nil
}

func (r users) Delete(_ context.Context, id string) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if _, ok := r.w.users[id]; !ok {
		return domain.ErrUserNotFound
	}
	delete(r.w.users, id)
	for k, t := range r.w.tokens {
		if t.UserID == id {
			delete(r.w.tokens, k)
		}
	}
	return nil
}

// --- refresh tokens ---

type tokens struct{ w *World }

func (r tokens) Create(_ context.Context, t domain.RefreshToken) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.tokens[t.ID] = t
	return nil
}

func (r tokens) FindByHash(_ context.Context, hash string) (domain.RefreshToken, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, t := range r.w.tokens {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return domain.RefreshToken{}, domain.ErrRefreshTokenInvalid
}

func (r tokens) Revoke(_ context.Context, id string, at time.Time) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	t, ok := r.w.tokens[id]
	if !ok {
		return domain.ErrRefreshTokenInvalid
	}
	if t.RevokedAt != nil {
		return domain.ErrRefreshTokenReused
	}
	t.RevokedAt = &at
	r.w.tokens[id] = t
	return nil
}

func (r tokens) Delete(_ context.Context, id string) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	delete(r.w.tokens, id)
	return nil
}

func (r tokens) DeleteAllForUser(_ context.Context, userID string) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for k, t := range r.w.tokens {
		if t.UserID == userID {
			delete(r.w.tokens, k)
		}
	}
	return nil
}

// --- hasher ---

// Hasher é um hasher reversível e rápido, só para teste.
type Hasher struct{}

func (Hasher) Hash(plain string) (string, error) { return "hashed:" + plain, nil }
func (Hasher) Verify(plain, hash string) bool    { return hash == "hashed:"+plain }

// ErrBoom é um erro qualquer para simular falha de infraestrutura.
var ErrBoom = errors.New("fakes: falha simulada")
