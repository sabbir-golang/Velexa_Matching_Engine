// Package dbtest gives tests a fresh PostgreSQL database. Set ME_TEST_DB_URL to
// a server where the user may create databases (make test-db starts one in a
// throwaway container); without it, database tests are skipped.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velexa-sim-me/internal/database/db"
)

// Open creates an empty, migrated database for this test and drops it when
// the test ends. It returns the pool and the database's URL.
func Open(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	base := os.Getenv("ME_TEST_DB_URL")
	if base == "" {
		t.Skip("ME_TEST_DB_URL not set — skipping PostgreSQL test (run make test-db)")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("ME_TEST_DB_URL: %v", err)
	}
	b := make([]byte, 6)
	rand.Read(b)
	name := "me_test_" + hex.EncodeToString(b)
	u.Path = "/" + name
	dsn := u.String()
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn, 4)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		u.Path = "/postgres"
		if conn, err := pgx.Connect(ctx, u.String()); err == nil {
			conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
			conn.Close(ctx)
		}
	})
	return pool, dsn
}
