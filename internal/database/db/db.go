// Package db connects the engine to PostgreSQL (ME_DB_URL), where console
// accounts and the audit trail are kept. Without ME_DB_URL the engine uses its
// JSON files instead and this package is not used.
package db

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// connectWait is how long Open keeps retrying while the server starts up
// (a container started at the same time as the engine, for example).
var connectWait = 30 * time.Second

// Open connects to the database at dsn, creates the database if it does not
// exist yet, applies the migrations, and returns a connection pool.
func Open(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("ME_DB_URL is not a valid PostgreSQL URL: %w", redactErr(err, dsn))
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	if err := ensureDatabase(ctx, cfg.ConnConfig); err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, redactErr(err, dsn)
	}
	if err := waitPing(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("cannot reach %s: %w", Describe(dsn), redactErr(err, dsn))
	}
	if err := Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func waitPing(ctx context.Context, pool *pgxpool.Pool) error {
	deadline := time.Now().Add(connectWait)
	for {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := pool.Ping(pctx)
		cancel()
		if err == nil || time.Now().After(deadline) || isFatal(err) {
			return err
		}
		time.Sleep(time.Second)
	}
}

// ensureDatabase creates the target database when the server says it does not
// exist (SQLSTATE 3D000), connecting to the server's "postgres" database to do so.
func ensureDatabase(ctx context.Context, cc *pgx.ConnConfig) error {
	deadline := time.Now().Add(connectWait)
	for {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		conn, err := pgx.ConnectConfig(cctx, cc)
		cancel()
		if err == nil {
			return conn.Close(ctx)
		}
		if sqlState(err) == "3D000" {
			return createDatabase(ctx, cc)
		}
		if isFatal(err) || time.Now().After(deadline) {
			return fmt.Errorf("cannot connect to PostgreSQL at %s:%d as %q: %v", cc.Host, cc.Port, cc.User, err)
		}
		time.Sleep(time.Second)
	}
}

func createDatabase(ctx context.Context, cc *pgx.ConnConfig) error {
	admin := cc.Copy()
	admin.Database = "postgres"
	conn, err := pgx.ConnectConfig(ctx, admin)
	if err != nil {
		return fmt.Errorf("database %q does not exist and the server's \"postgres\" database is not reachable to create it: %v", cc.Database, err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{cc.Database}.Sanitize()); err != nil {
		return fmt.Errorf("database %q does not exist and could not be created (the user needs CREATEDB, or create it by hand): %v", cc.Database, err)
	}
	log.Printf("[db] created database %q", cc.Database)
	return nil
}

func sqlState(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// isFatal reports errors that waiting will not fix: wrong password, missing
// role, bad TLS settings.
func isFatal(err error) bool {
	switch sqlState(err) {
	case "28P01", "28000", "3D000", "42501":
		return true
	}
	return strings.Contains(err.Error(), "SSL")
}

// Describe names the database of a URL for logs, without the password:
// "velexa_me@127.0.0.1:5435/velexa_me".
func Describe(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return "(database)"
	}
	user := ""
	if u.User != nil {
		user = u.User.Username() + "@"
	}
	return user + u.Host + u.Path
}

// redactErr removes the password from an error that may quote the URL.
func redactErr(err error, dsn string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if u, perr := url.Parse(dsn); perr == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok && pw != "" {
			msg = strings.ReplaceAll(msg, pw, "***")
		}
	}
	return errors.New(msg)
}
