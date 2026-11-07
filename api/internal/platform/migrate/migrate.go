// Package migrate aplica as migrations goose embutidas no binário. Roda só
// no modo `migrate`, num container de execução única antes da API e do
// worker (ver docs/arquitetura.md).
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx" para database/sql
	"github.com/pressly/goose/v3"
)

// Up aplica todas as migrations pendentes de migrations em url.
func Up(ctx context.Context, url string, migrations fs.FS) error {
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
