// Package migrate aplica as migrations goose embutidas no binário e as do
// River (fila de jobs). Roda só no modo `migrate`, num container de
// execução única antes da API e do worker (ver docs/arquitetura.md).
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx" para database/sql
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/mphennrichs/nelisa/api/internal/platform/jobs"
)

// Up aplica todas as migrations pendentes de migrations em url e, em
// seguida, as do River no schema jobs.Schema (criado pela migration
// inicial). As duas etapas são idempotentes.
func Up(ctx context.Context, url string, migrations fs.FS) error {
	if err := gooseUp(ctx, url, migrations); err != nil {
		return err
	}
	return riverUp(ctx, url)
}

func gooseUp(ctx context.Context, url string, migrations fs.FS) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("abrindo banco: %w", err)
	}
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations)
	if err != nil {
		return fmt.Errorf("preparando migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("aplicando migrations: %w", err)
	}
	return nil
}

func riverUp(ctx context.Context, url string) error {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("abrindo banco para o river: %w", err)
	}
	defer pool.Close()
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Schema: jobs.Schema})
	if err != nil {
		return fmt.Errorf("preparando migrations do river: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("aplicando migrations do river: %w", err)
	}
	return nil
}
