// Package auth é a parte compartilhada da autenticação: o Ator (quem faz a
// requisição) levado no contexto, as roles e o token de acesso (JWT).
//
// Quem valida a sessão e põe o Ator no contexto é o middleware do módulo
// team (team/httpapi). Qualquer módulo lê o Ator daqui com ActorFrom e
// decide no próprio caso de uso se ele pode agir. As regras do cadastro
// (quem existe, que role tem) continuam no team; aqui só fica o vocabulário
// comum a todos os módulos.
package auth

import (
	"context"
	"net/http"

	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
)

// Role é a role da equipe. Editor ⊂ Revisor ⊂ Admin (CONTEXT.md): cada uma
// pode tudo o que a anterior pode. Os valores são os mesmos de
// team/domain.Role, que é a fonte da verdade do cadastro.
type Role string

const (
	RoleEditor   Role = "editor"
	RoleReviewer Role = "reviewer"
	RoleAdmin    Role = "admin"
)

var rank = map[Role]int{RoleEditor: 1, RoleReviewer: 2, RoleAdmin: 3}

// AtLeast diz se r inclui min. Role desconhecida não inclui nada.
func (r Role) AtLeast(min Role) bool {
	got, ok := rank[r]
	return ok && got >= rank[min]
}

// Actor é a pessoa da equipe autenticada na requisição.
type Actor struct {
	UserID string
	Role   Role
}

type actorKey struct{}

// WithActor devolve um contexto que carrega a.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom devolve o Ator do contexto; ok é false fora de uma requisição
// autenticada.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok && a.UserID != ""
}

// RequireRole recusa cedo (401 sem Ator, 403 com role abaixo de min). Vai
// depois do middleware de autenticação. É só a primeira barreira: o caso
// de uso confere de novo.
func RequireRole(min Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a, ok := ActorFrom(r.Context())
			if !ok {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Faça login para continuar.")
				return
			}
			if !a.Role.AtLeast(min) {
				httpx.WriteError(w, http.StatusForbidden, "forbidden", "Você não tem permissão para isso.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
