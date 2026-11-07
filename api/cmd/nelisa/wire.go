package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	auditapp "github.com/mphennrichs/nelisa/api/internal/audit/app"
	auditstore "github.com/mphennrichs/nelisa/api/internal/audit/store"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clientip"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/config"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/platform/password"
	teamapp "github.com/mphennrichs/nelisa/api/internal/team/app"
	teamhttp "github.com/mphennrichs/nelisa/api/internal/team/httpapi"
	teamstore "github.com/mphennrichs/nelisa/api/internal/team/store"
)

// teamPorts são as implementações que o módulo team recebe. Em produção vêm
// do Postgres; nos testes do roteador, dos fakes.
type teamPorts struct {
	users  teamapp.UserRepo
	tokens teamapp.RefreshTokenRepo
	tx     teamapp.TxRunner
	audit  teamapp.AuditRecorder
	hasher teamapp.PasswordHasher
}

func postgresTeamPorts(pool *pgxpool.Pool, clk clock.Clock) teamPorts {
	return teamPorts{
		users:  teamstore.NewUsers(pool),
		tokens: teamstore.NewRefreshTokens(pool),
		tx:     db.NewTxRunner(pool),
		audit:  auditRecorder{uc: auditapp.NewRecordEvent(auditstore.New(pool), clk)},
		hasher: password.NewBcrypt(),
	}
}

// auditRecorder liga a porta AuditRecorder do team ao caso de uso do módulo
// audit. Os dois módulos não se conhecem; só o main conhece os dois.
type auditRecorder struct{ uc *auditapp.RecordEvent }

func (a auditRecorder) Record(ctx context.Context, e teamapp.AuditEvent) error {
	return a.uc.Execute(ctx, auditapp.RecordEventInput{
		ActorID: e.ActorID, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID, Metadata: e.Metadata,
	})
}

// teamModule é o que o roteador precisa do team.
type teamModule struct {
	handlers *teamhttp.Handlers
	// authenticate valida o token de acesso e põe o Ator no contexto; vale
	// para toda rota de /api/team fora de /auth.
	authenticate func(http.Handler) http.Handler
}

func newTeamModule(cfg config.Config, p teamPorts, clk clock.Clock, logger *slog.Logger) teamModule {
	access := auth.NewTokens(cfg.JWTSecret, clk)
	sessions := teamapp.NewSessions(access, p.tokens, clk)
	authenticate := teamapp.NewAuthenticate(access, p.users)
	uc := teamhttp.UseCases{
		Bootstrap:      teamapp.NewBootstrap(p.users, p.hasher, sessions, p.tx, p.audit, clk, cfg.BootstrapToken),
		Login:          teamapp.NewLogin(p.users, p.hasher, sessions),
		Refresh:        teamapp.NewRefresh(p.tokens, p.users, sessions, clk),
		Logout:         teamapp.NewLogout(p.tokens),
		Authenticate:   authenticate,
		GetMe:          teamapp.NewGetMe(p.users),
		ChangePassword: teamapp.NewChangePassword(p.users, p.tokens, p.hasher, sessions, p.tx, clk),
		CreateUser:     teamapp.NewCreateUser(p.users, p.hasher, p.tx, p.audit, clk),
		ResetPassword:  teamapp.NewResetPassword(p.users, p.tokens, p.hasher, p.tx, p.audit, clk),
		UpdateRole:     teamapp.NewUpdateRole(p.users, p.tx, p.audit),
		DeleteUser:     teamapp.NewDeleteUser(p.users, p.tx, p.audit),
		ListUsers:      teamapp.NewListUsers(p.users),
	}
	resolver := clientip.New(cfg.TrustedProxies)
	return teamModule{
		handlers: teamhttp.New(uc, teamhttp.Options{
			SecureCookies: cfg.SecureCookies,
			LimitKey:      resolver.LimitKey,
		}, logger),
		authenticate: teamhttp.Authenticator(authenticate, logger),
	}
}
