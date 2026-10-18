package users

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgBackend keeps the accounts in the PostgreSQL table users (see
// internal/database/db/migrations). Each change is one upsert or delete.
type pgBackend struct {
	pool *pgxpool.Pool
	name string
}

const pgTimeout = 5 * time.Second

const userCols = `username, role, disabled, password_hash, session_ver,
	full_name, email, phone, gender, country, created_at, last_login_at`

// dbExec is what an upsert needs: a pool or a transaction.
type dbExec interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (p *pgBackend) describe() string { return p.name }

func (p *pgBackend) load() ([]*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, "SELECT "+userCols+" FROM users ORDER BY username_key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		var u User
		var role string
		var created time.Time
		var last *time.Time
		if err := rows.Scan(&u.Username, &role, &u.Disabled, &u.Hash, &u.SessionVer,
			&u.FullName, &u.Email, &u.Phone, &u.Gender, &u.Country, &created, &last); err != nil {
			return nil, err
		}
		u.Role = Role(role)
		u.Created = created.Local().Format(timeFormat)
		if last != nil {
			u.LastLogin = last.Local().Format(timeFormat)
		}
		out = append(out, &u)
	}
	return out, rows.Err()
}

func (p *pgBackend) save(all map[string]*User, k string) error {
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	if u := all[k]; u != nil {
		return upsertUser(ctx, p.pool, u)
	}
	_, err := p.pool.Exec(ctx, "DELETE FROM users WHERE username_key = $1", k)
	return err
}

func upsertUser(ctx context.Context, db dbExec, u *User) error {
	_, err := db.Exec(ctx, `INSERT INTO users (username_key, `+userCols+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (username_key) DO UPDATE SET
			username = EXCLUDED.username, role = EXCLUDED.role, disabled = EXCLUDED.disabled,
			password_hash = EXCLUDED.password_hash, session_ver = EXCLUDED.session_ver,
			full_name = EXCLUDED.full_name, email = EXCLUDED.email, phone = EXCLUDED.phone,
			gender = EXCLUDED.gender, country = EXCLUDED.country,
			last_login_at = EXCLUDED.last_login_at, updated_at = now()`,
		key(u.Username), u.Username, string(u.Role), u.Disabled, u.Hash, u.SessionVer,
		u.FullName, u.Email, u.Phone, u.Gender, u.Country, parseTime(u.Created, time.Now()), optTime(u.LastLogin))
	return err
}

// importFile copies the accounts of an older users.json into an empty users
// table, keeping password hashes and session tokens (nobody has to sign in
// again), then renames the file so it is not imported twice. It does nothing
// when the table already has accounts or there is no file.
func (p *pgBackend) importFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var n int
	if err := p.pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []*User
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("%s is unreadable, not imported: %w", path, err)
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	imported := 0
	for _, u := range list {
		if u == nil || u.Username == "" || !u.Role.Valid() {
			continue
		}
		if u.SessionVer == "" {
			u.SessionVer = newToken()
		}
		if err := upsertUser(ctx, tx, u); err != nil {
			return fmt.Errorf("importing %q: %w", u.Username, err)
		}
		imported++
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	done := path + ".imported-" + time.Now().Format("20060102-150405")
	if err := os.Rename(path, done); err != nil {
		log.Printf("[users] imported %d accounts from %s, but could not rename it: %v", imported, path, err)
		return nil
	}
	log.Printf("[users] imported %d accounts from %s into the database (file kept as %s)", imported, path, done)
	return nil
}

func parseTime(s string, def time.Time) time.Time {
	if t, err := time.ParseInLocation(timeFormat, s, time.Local); err == nil {
		return t
	}
	return def
}

func optTime(s string) *time.Time {
	if t, err := time.ParseInLocation(timeFormat, s, time.Local); err == nil {
		return &t
	}
	return nil
}
