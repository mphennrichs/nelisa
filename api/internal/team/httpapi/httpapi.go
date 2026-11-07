// Package httpapi é o adaptador HTTP do módulo team: login, sessão, a
// própria conta e a administração da equipe, sob /api/team. Também tem o
// middleware que autentica toda rota /api/team.
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
	"github.com/mphennrichs/nelisa/api/internal/team/app"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

const (
	// RefreshCookieName é o cookie da sessão. Só viaja para as rotas de
	// /api/team/auth (Path), nunca para o resto da API.
	RefreshCookieName = "nelisa_refresh"
	RefreshCookiePath = "/api/team/auth"

	// Limites por IP. Login e bootstrap são alvo de força bruta; 10/min
	// deixa espaço para algumas senhas digitadas errado. O refresh é
	// chamado sozinho pelo front a cada expiração do token de acesso: balde
	// próprio e mais folgado.
	loginPerMinute   = 10
	refreshPerMinute = 30
)

// UseCases reúne os casos de uso que o HTTP dirige.
type UseCases struct {
	Bootstrap      *app.Bootstrap
	Login          *app.Login
	Refresh        *app.Refresh
	Logout         *app.Logout
	Authenticate   *app.Authenticate
	GetMe          *app.GetMe
	ChangePassword *app.ChangePassword
	CreateUser     *app.CreateUser
	ResetPassword  *app.ResetPassword
	UpdateRole     *app.UpdateRole
	DeleteUser     *app.DeleteUser
	ListUsers      *app.ListUsers
}

// Options são as configurações de implantação da camada HTTP.
type Options struct {
	// SecureCookies marca o cookie como Secure. O TLS termina no proxy,
	// então a requisição nunca parece https: true em produção, false só em
	// dev sobre http (config.SecureCookies).
	SecureCookies bool
	// LimitKey é a chave do rate limit por IP (clientip.Resolver.LimitKey).
	LimitKey func(*http.Request) (string, error)
}

// Handlers são os handlers HTTP do team.
type Handlers struct {
	uc     UseCases
	opts   Options
	logger *slog.Logger
}

func New(uc UseCases, opts Options, logger *slog.Logger) *Handlers {
	return &Handlers{uc: uc, opts: opts, logger: logger}
}

// RegisterAuth monta as rotas públicas de sessão em r (o roteador de
// /api/team): são as únicas de /api/team sem autenticação.
func (h *Handlers) RegisterAuth(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		loginLimit := httprate.Limit(loginPerMinute, time.Minute,
			httprate.WithKeyFuncs(h.opts.LimitKey), httprate.WithLimitHandler(tooManyRequests))
		r.With(loginLimit).Post("/bootstrap", h.bootstrap)
		r.With(loginLimit).Post("/login", h.login)
		r.With(httprate.Limit(refreshPerMinute, time.Minute,
			httprate.WithKeyFuncs(h.opts.LimitKey), httprate.WithLimitHandler(tooManyRequests))).
			Post("/refresh", h.refresh)
		r.Post("/logout", h.logout)
	})
}

// RegisterAuthenticated monta as rotas que exigem login. r já deve estar
// atrás de Authenticator.
func (h *Handlers) RegisterAuthenticated(r chi.Router) {
	r.Get("/me", h.getMe)
	r.Post("/me/password", h.changePassword)
	r.Route("/users", func(r chi.Router) {
		// Recusa cedo; os casos de uso conferem de novo.
		r.Use(auth.RequireRole(auth.RoleAdmin))
		r.Get("/", h.listUsers)
		r.Post("/", h.createUser)
		r.Patch("/{id}", h.updateRole)
		r.Post("/{id}/password", h.resetPassword)
		r.Delete("/{id}", h.deleteUser)
	})
}

// Authenticator valida `Authorization: Bearer <token de acesso>` e põe o
// Ator no contexto. Toda rota de /api/team fora de /auth passa por ele.
func Authenticator(uc *app.Authenticate, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Faça login para continuar.")
				return
			}
			actor, err := uc.Execute(r.Context(), app.AuthenticateInput{AccessToken: token})
			if err != nil {
				writeErr(w, r, logger, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithActor(r.Context(), actor)))
		})
	}
}

// --- sessão ---

type sessionDTO struct {
	AccessToken string  `json:"access_token"`
	TokenType   string  `json:"token_type"`
	ExpiresAt   string  `json:"expires_at"`
	User        userDTO `json:"user"`
}

type userDTO struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Email              string `json:"email"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
	CreatedAt          string `json:"created_at"`
}

func toUserDTO(u domain.User) userDTO {
	return userDTO{
		ID: u.ID, Name: u.Name, Email: u.Email, Role: string(u.Role),
		MustChangePassword: u.MustChangePassword, CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// writeSession põe o refresh token no cookie e o token de acesso no corpo:
// o front guarda o de acesso só em memória; o cookie HttpOnly vai sozinho
// para /api/team/auth e o JavaScript nunca o lê.
func (h *Handlers) writeSession(w http.ResponseWriter, status int, s app.Session) {
	http.SetCookie(w, &http.Cookie{
		Name: RefreshCookieName, Value: s.RefreshToken, Path: RefreshCookiePath,
		MaxAge: int(domain.RefreshTokenTTL.Seconds()), Expires: s.RefreshExpiresAt,
		HttpOnly: true, Secure: h.opts.SecureCookies, SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, status, sessionDTO{
		AccessToken: s.AccessToken, TokenType: "Bearer",
		ExpiresAt: s.AccessExpiresAt.UTC().Format(time.RFC3339), User: toUserDTO(s.User),
	})
}

func (h *Handlers) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: RefreshCookieName, Value: "", Path: RefreshCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: h.opts.SecureCookies, SameSite: http.SameSiteStrictMode,
	})
}

func refreshFromCookie(r *http.Request) string {
	c, err := r.Cookie(RefreshCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func (h *Handlers) bootstrap(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	s, err := h.uc.Bootstrap.Execute(r.Context(), app.BootstrapInput(req))
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	h.writeSession(w, http.StatusCreated, s)
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	s, err := h.uc.Login.Execute(r.Context(), app.LoginInput(req))
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	h.writeSession(w, http.StatusOK, s)
}

func (h *Handlers) refresh(w http.ResponseWriter, r *http.Request) {
	s, err := h.uc.Refresh.Execute(r.Context(), app.RefreshInput{RefreshToken: refreshFromCookie(r)})
	switch {
	case errors.Is(err, domain.ErrRefreshTokenInvalid), errors.Is(err, domain.ErrRefreshTokenExpired),
		errors.Is(err, domain.ErrRefreshTokenReused):
		h.clearRefreshCookie(w)
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "Sessão encerrada. Faça login de novo.")
	case err != nil:
		writeErr(w, r, h.logger, err)
	default:
		h.writeSession(w, http.StatusOK, s)
	}
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.Logout.Execute(r.Context(), app.LogoutInput{RefreshToken: refreshFromCookie(r)}); err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// --- a própria conta ---

func (h *Handlers) getMe(w http.ResponseWriter, r *http.Request) {
	u, err := h.uc.GetMe.Execute(r.Context(), app.GetMeInput{})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toUserDTO(u))
}

func (h *Handlers) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	s, err := h.uc.ChangePassword.Execute(r.Context(), app.ChangePasswordInput(req))
	if errors.Is(err, app.ErrInvalidCredentials) {
		// 403, não 401: a sessão continua válida, só a senha atual é que
		// não confere; o front não deve tratar como "deslogado".
		httpx.WriteError(w, http.StatusForbidden, "wrong_password", "A senha atual não confere.")
		return
	}
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	// Todas as sessões acabaram, inclusive esta: entrega a nova.
	h.writeSession(w, http.StatusOK, s)
}

// --- administração da equipe ---

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.uc.ListUsers.Execute(r.Context(), app.ListUsersInput{})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	out := make([]userDTO, 0, len(users))
	for _, u := range users {
		out = append(out, toUserDTO(u))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (h *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	u, err := h.uc.CreateUser.Execute(r.Context(), app.CreateUserInput(req))
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toUserDTO(u))
}

func (h *Handlers) updateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role string `json:"role"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	u, err := h.uc.UpdateRole.Execute(r.Context(), app.UpdateRoleInput{UserID: chi.URLParam(r, "id"), Role: req.Role})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toUserDTO(u))
}

func (h *Handlers) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	err := h.uc.ResetPassword.Execute(r.Context(), app.ResetPasswordInput{UserID: chi.URLParam(r, "id"), NewPassword: req.Password})
	if err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.uc.DeleteUser.Execute(r.Context(), app.DeleteUserInput{UserID: chi.URLParam(r, "id")}); err != nil {
		writeErr(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- erros ---

type errMapping struct {
	err     error
	status  int
	code    string
	message string
}

// errMappings traduz erros de domínio e de caso de uso para o formato
// padrão. A mensagem vai para a tela.
func errMappings() []errMapping {
	return []errMapping{
		{app.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated", "Faça login para continuar."},
		{app.ErrSessionExpired, http.StatusUnauthorized, "session_expired", "Sua sessão foi encerrada. Faça login de novo."},
		{app.ErrForbidden, http.StatusForbidden, "forbidden", "Você não tem permissão para isso."},
		{app.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials", "E-mail ou senha incorretos."},
		{app.ErrBootstrapDisabled, http.StatusForbidden, "bootstrap_disabled", "O cadastro inicial está desligado."},
		{app.ErrBootstrapTokenInvalid, http.StatusForbidden, "bootstrap_token_invalid", "Token de cadastro inicial incorreto."},
		{app.ErrAlreadyBootstrapped, http.StatusConflict, "already_bootstrapped", "O cadastro inicial já foi feito."},
		{domain.ErrUserNotFound, http.StatusNotFound, "user_not_found", "Usuário não encontrado."},
		{domain.ErrEmailTaken, http.StatusConflict, "email_taken", "Já existe alguém com esse e-mail."},
		{domain.ErrInvalidEmail, http.StatusBadRequest, "invalid_email", "E-mail inválido."},
		{domain.ErrInvalidName, http.StatusBadRequest, "invalid_name", "Informe um nome com até 120 caracteres."},
		{domain.ErrInvalidRole, http.StatusBadRequest, "invalid_role", "Role inválida: use editor, reviewer ou admin."},
		{domain.ErrPasswordTooShort, http.StatusBadRequest, "password_too_short", "A senha precisa de pelo menos 8 caracteres."},
		{domain.ErrPasswordTooLong, http.StatusBadRequest, "password_too_long", "A senha pode ter no máximo 72 bytes."},
		{domain.ErrLastAdmin, http.StatusConflict, "last_admin", "Precisa sobrar ao menos um Admin."},
		{domain.ErrCannotDeleteSelf, http.StatusConflict, "cannot_delete_self", "Você não pode excluir a própria conta."},
		{domain.ErrCredentialsChanged, http.StatusConflict, "credentials_changed", "A senha mudou nesse meio-tempo. Tente de novo."},
	}
}

func writeErr(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	for _, m := range errMappings() {
		if errors.Is(err, m.err) {
			httpx.WriteError(w, m.status, m.code, m.message)
			return
		}
	}
	logger.ErrorContext(r.Context(), "team request failed", "error", err.Error())
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Erro interno. Tente de novo.")
}

func tooManyRequests(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteError(w, http.StatusTooManyRequests, "too_many_requests", "Muitas tentativas. Espere um minuto e tente de novo.")
}
