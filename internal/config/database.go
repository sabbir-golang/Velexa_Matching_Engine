package config

import "strconv"

// DBURL is the PostgreSQL URL where console accounts and the audit trail are
// kept (ME_DB_URL), e.g. postgres://velexa_me:secret@127.0.0.1:5435/velexa_me?sslmode=disable.
// "" means no database: the engine uses users.json and admin_audit.jsonl.
func DBURL() string { return EnvOr("ME_DB_URL", "") }

// DBMaxConns is the database connection-pool size (ME_DB_MAX_CONNS, default 10).
func DBMaxConns() int32 {
	if n, err := strconv.Atoi(EnvOr("ME_DB_MAX_CONNS", "10")); err == nil && n > 0 && n <= 200 {
		return int32(n)
	}
	return 10
}
