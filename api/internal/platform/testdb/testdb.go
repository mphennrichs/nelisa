// Package testdb dá aos testes de integração um banco Postgres limpo e
// migrado. Usa NELISA_TEST_DATABASE_URL (o CI sobe um Postgres de serviço;
// localmente, `docker compose -f api/compose.dev.yaml up -d`). Sem a
// variável, o teste é pulado.
package testdb

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/platform/migrate"
	"github.com/mphennrichs/nelisa/api/migrations"
)

// New cria um banco descartável, aplica as migrations e devolve o pool e a
// URL. O banco é apagado ao fim do teste.
func New(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	admin := os.Getenv("NELISA_TEST_DATABASE_URL")
	if admin == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("NELISA_TEST_DATABASE_URL é obrigatório no CI")
		}
		t.Skip("NELISA_TEST_DATABASE_URL não definido; pulando teste de integração")
	}
	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, admin)
	if err != nil {
		t.Fatalf("conectando ao postgres de teste: %v", err)
	}
	name := fmt.Sprintf("nelisa_test_%d", rand.Uint64())
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		adminPool.Close()
		t.Fatalf("criando banco de teste: %v", err)
	}
	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("NELISA_TEST_DATABASE_URL inválido: %v", err)
	}
	u.Path = "/" + name
	dbURL := u.String()

	if err := migrate.Up(ctx, dbURL, migrations.FS); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("abrindo banco de teste: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = adminPool.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		adminPool.Close()
	})
	return pool, dbURL
}
