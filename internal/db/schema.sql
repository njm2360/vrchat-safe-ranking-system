PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS issued_tokens (
    jti          TEXT PRIMARY KEY,
    discord_id   TEXT NOT NULL REFERENCES users(discord_id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    jwt          TEXT NOT NULL,
    issued_at    TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_issued_tokens_discord ON issued_tokens(discord_id, issued_at DESC);

CREATE TABLE IF NOT EXISTS users (
    discord_id   TEXT PRIMARY KEY,
    display_name TEXT NOT NULL UNIQUE,
    current_jti  TEXT REFERENCES issued_tokens(jti),
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS jti_blacklist (
    jti        TEXT PRIMARY KEY,
    reason     TEXT,
    created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS discord_bans (
    discord_id TEXT PRIMARY KEY,
    reason     TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS display_name_bans (
    display_name TEXT PRIMARY KEY,
    reason       TEXT,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS save_history (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    display_name TEXT NOT NULL,
    score        INTEGER NOT NULL,
    created_at   TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_save_history_dn ON save_history(display_name, id DESC);

CREATE TABLE IF NOT EXISTS latest_saves (
    display_name TEXT PRIMARY KEY,
    score        INTEGER NOT NULL,
    history_id   INTEGER NOT NULL REFERENCES save_history(id),
    jti          TEXT REFERENCES issued_tokens(jti) ON DELETE SET NULL,
    updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_latest_saves_score ON latest_saves(score DESC);

CREATE TABLE IF NOT EXISTS oauth_states (
    state         TEXT PRIMARY KEY,
    proposed_name TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL,
    expires_at    TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_oauth_states_expires ON oauth_states(expires_at);

CREATE TABLE IF NOT EXISTS auth_sessions (
    token            TEXT PRIMARY KEY,
    discord_id       TEXT NOT NULL,
    discord_username TEXT NOT NULL,
    proposed_name    TEXT NOT NULL,
    created_at       TIMESTAMP NOT NULL,
    expires_at       TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires ON auth_sessions(expires_at);
