package db_test

import (
	"context"
	"testing"

	"velexa-sim-me/internal/database/db"
	"velexa-sim-me/internal/database/db/dbtest"
)

func TestMigrationsApplyOnceAndInOrder(t *testing.T) {
	pool, dsn := dbtest.Open(t) // Open created the database and migrated it
	ctx := context.Background()
	if v := db.SchemaVersion(ctx, pool); v != 1 {
		t.Fatalf("schema version %d, want 1", v)
	}
	if err := db.Migrate(ctx, pool); err != nil { // second run: nothing to do
		t.Fatal(err)
	}
	var n int
	pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&n)
	if n != 1 {
		t.Fatalf("%d migration rows after a second run, want 1", n)
	}
	for _, table := range []string{"users", "audit_log"} {
		var ok bool
		pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&ok)
		if !ok {
			t.Errorf("table %s missing", table)
		}
	}
	if db.Describe(dsn) == "" || db.Describe("postgres://u:secret@h:1/x") != "u@h:1/x" {
		t.Fatalf("Describe must name the database without the password: %q", db.Describe("postgres://u:secret@h:1/x"))
	}
}
