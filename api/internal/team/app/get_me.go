package app

import (
	"context"
	"errors"

	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type GetMeInput struct{}

// GetMe devolve o cadastro do próprio Ator.
type GetMe struct {
	users UserRepo
}

func NewGetMe(users UserRepo) *GetMe { return &GetMe{users: users} }

func (uc *GetMe) Execute(ctx context.Context, _ GetMeInput) (domain.User, error) {
	actor, err := currentActor(ctx)
	if err != nil {
		return domain.User{}, err
	}
	u, err := uc.users.FindByID(ctx, actor.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return domain.User{}, ErrUnauthenticated
	}
	return u, err
}
