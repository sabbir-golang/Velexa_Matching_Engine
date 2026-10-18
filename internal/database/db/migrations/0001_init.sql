-- Console accounts and the audit trail.

CREATE TABLE users (
    username_key  text PRIMARY KEY,                 -- lower-case username: names are case-insensitive
    username      text NOT NULL,                    -- as the user typed it
    role          text NOT NULL CHECK (role IN ('admin', 'user')),
    disabled      boolean NOT NULL DEFAULT false,
    password_hash text NOT NULL,                    -- pbkdf2-sha256$iterations$salt$hash
    session_ver   text NOT NULL,                    -- changing it ends every session of the account
    full_name     text NOT NULL DEFAULT '',
    email         text NOT NULL DEFAULT '',
    phone         text NOT NULL DEFAULT '',
    gender        text NOT NULL DEFAULT '',
    country       text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz,
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_key ON users (lower(email)) WHERE email <> '';

CREATE TABLE audit_log (
    id     bigserial PRIMARY KEY,
    at     timestamptz NOT NULL DEFAULT now(),
    actor  text NOT NULL,   -- "alice (10.0.0.5)"
    action text NOT NULL,   -- ISSUE, REVOKE, user.add, login.ok, ...
    target text NOT NULL DEFAULT ''
);

CREATE INDEX audit_log_at ON audit_log (at DESC);
CREATE INDEX audit_log_actor ON audit_log (lower(actor) text_pattern_ops, at DESC);
