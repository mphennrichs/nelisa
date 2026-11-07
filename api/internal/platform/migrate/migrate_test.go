package migrate_test

import (
	"context"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/platform/migrate"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
	"github.com/mphennrichs/nelisa/api/migrations"
)

func TestUpIsIdempotentAndCreatesSchemas(t *testing.T) {
	pool, url := testdb.New(t) // already migrated once
	if err := migrate.Up(context.Background(), url, migrations.FS); err != nil {
		t.Fatalf("second run should be a no-op: %v", err)
	}
	for _, schema := range []string{"team", "audit"} {
		var ok bool
		err := pool.QueryRow(context.Background(),
			"SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)", schema).Scan(&ok)
		if err != nil || !ok {
			t.Errorf("schema %s missing (err=%v)", schema, err)
		}
	}
}
