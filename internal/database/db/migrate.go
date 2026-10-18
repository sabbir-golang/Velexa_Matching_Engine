package db

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Schema changes are numbered SQL files in migrations/ ("0002_add_x.sql").
// Each runs once, in order, in its own transaction, and is recorded in
// schema_migrations. Never edit a file that has been released: add a new one.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLock is the advisory-lock key that keeps two engines starting at the
// same time from migrating at once.
const migrationLock = 0x76656c6578 // "velex"

type migration struct {
	version int
	name    string
	sql     string
}

func migrations() ([]migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		num, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || !strings.HasSuffix(e.Name(), ".sql") {
			return nil, fmt.Errorf("migration file %q must be named NNNN_description.sql", e.Name())
		}
		b, err := migrationFiles.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: e.Name(), sql: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("two migrations with version %d", out[i].version)
		}
	}
	return out, nil
}

// Migrate applies every migration the database has not seen yet and returns
// nothing when it is up to date.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	ms, err := migrations()
	if err != nil {
		return err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLock); err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLock)

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    integer PRIMARY KEY,
		name       text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}
	done := map[int]bool{}
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		done[v] = true
	}
	rows.Close()

	for _, m := range ms {
		if done[m.version] {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", m.version, m.name); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		log.Printf("[db] applied migration %s", m.name)
	}
	return nil
}

// SchemaVersion is the newest migration applied to the database.
func SchemaVersion(ctx context.Context, pool *pgxpool.Pool) int {
	var v int
	_ = pool.QueryRow(ctx, "SELECT coalesce(max(version), 0) FROM schema_migrations").Scan(&v)
	return v
}
