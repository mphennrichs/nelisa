package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type ListUsersInput struct{}

// ListUsers lista a equipe para o Admin.
type ListUsers struct {
	users UserRepo
}

func NewListUsers(users UserRepo) *ListUsers { return &ListUsers{users: users} }

func (uc *ListUsers) Execute(ctx context.Context, _ ListUsersInput) ([]domain.User, error) {
	if _, err := requireUserManager(ctx); err != nil {
		return nil, err
	}
	return uc.users.List(ctx)
}
