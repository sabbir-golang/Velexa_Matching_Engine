package audit

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgSink keeps the audit trail in the PostgreSQL table audit_log.
type pgSink struct{ pool *pgxpool.Pool }

const pgTimeout = 2 * time.Second

// UseDB switches the audit trail to PostgreSQL. On first use it imports an
// existing admin_audit.jsonl (Path) into the empty table.
func UseDB(pool *pgxpool.Pool) error {
	p := &pgSink{pool: pool}
	if err := p.importFile(Path); err != nil {
		return err
	}
	use(p)
	return nil
}

func (p *pgSink) record(e Entry) {
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, "INSERT INTO audit_log (at, actor, action, target) VALUES ($1, $2, $3, $4)",
		e.Time, e.Actor, e.Action, e.Target)
	if err != nil { // keep the entry: write it to the file and say so
		log.Printf("[audit] database insert failed (%v) — entry kept in %s", err, Path)
		appendFile(Path, e)
	}
}

func (p *pgSink) recent(n int) []Entry { return p.query("", n) }

func (p *pgSink) recentFor(prefix string, n int) []Entry { return p.query(prefix, n) }

func (p *pgSink) query(prefix string, n int) []Entry {
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	var rows pgx.Rows
	var err error
	if prefix == "" {
		rows, err = p.pool.Query(ctx, "SELECT at, actor, action, target FROM audit_log ORDER BY at DESC, id DESC LIMIT $1", n)
	} else {
		rows, err = p.pool.Query(ctx, `SELECT at, actor, action, target FROM audit_log
			WHERE lower(actor) LIKE $1 ESCAPE '\' ORDER BY at DESC, id DESC LIMIT $2`, likePrefix(prefix), n)
	}
	out := []Entry{}
	if err != nil {
		log.Printf("[audit] query failed: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var e Entry
		if rows.Scan(&e.Time, &e.Actor, &e.Action, &e.Target) == nil {
			out = append(out, e)
		}
	}
	return out
}

// likePrefix makes a LIKE pattern that matches s literally at the start.
func likePrefix(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(strings.ToLower(s)) + "%"
}

// importFile bulk-loads the entries of admin_audit.jsonl into audit_log, then
// renames the file so they are not imported twice. On the first start with a
// database that is the whole old trail; later it is any entry that had to fall
// back to the file because an insert failed.
func (p *pgSink) importFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	entries, err := readFile(path)
	if os.IsNotExist(err) || (err == nil && len(entries) == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	src := pgx.CopyFromSlice(len(entries), func(i int) ([]any, error) {
		e := entries[i]
		return []any{e.Time, e.Actor, e.Action, e.Target}, nil
	})
	copied, err := p.pool.CopyFrom(ctx, pgx.Identifier{"audit_log"}, []string{"at", "actor", "action", "target"}, src)
	if err != nil {
		return fmt.Errorf("import %s: %w", path, err)
	}
	done := path + ".imported-" + time.Now().Format("20060102-150405")
	if err := os.Rename(path, done); err != nil {
		log.Printf("[audit] imported %d entries from %s, but could not rename it: %v", copied, path, err)
		return nil
	}
	log.Printf("[audit] imported %d entries from %s into the database (file kept as %s)", copied, path, done)
	return nil
}
