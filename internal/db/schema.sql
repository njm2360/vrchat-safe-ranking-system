PRAGMA foreign_keys = ON;

-- Registry of every JWT we have ever issued. Append-only.
-- This is the single source of truth for (jti -> discord_id, display_name, jwt).
-- Other tables reference issued_tokens.jti instead of duplicating discord_id.
CREATE TABLE IF NOT EXISTS issued_tokens (
    jti          TEXT PRIMARY KEY,
    discord_id   TEXT NOT NULL,
    display_name TEXT NOT NULL,
    jwt          TEXT NOT NULL,
    issued_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_issued_tokens_discord ON issued_tokens(discord_id, issued_at DESC);

-- Identity binding. current_jti points into issued_tokens.
CREATE TABLE IF NOT EXISTS users (
    discord_id   TEXT PRIMARY KEY,
    display_name TEXT NOT NULL UNIQUE,
    current_jti  TEXT REFERENCES issued_tokens(jti),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tickets (
    uuid         TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    issued_at    INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    consumed_at  INTEGER
);
CREATE INDEX IF NOT EXISTS idx_tickets_display_name ON tickets(display_name);

CREATE TABLE IF NOT EXISTS jti_blacklist (
    jti        TEXT PRIMARY KEY REFERENCES issued_tokens(jti),
    reason     TEXT,
    created_at INTEGER NOT NULL
);

-- Bans are by discord_id. Pre-emptive bans (before registration) are allowed,
-- so this does NOT FK to users.
CREATE TABLE IF NOT EXISTS bans (
    discord_id TEXT PRIMARY KEY,
    reason     TEXT,
    created_at INTEGER NOT NULL
);

-- Append-only save history. No JTI / discord_id stored here on purpose:
-- DisplayNames are globally unique and never reusable in VRChat.
CREATE TABLE IF NOT EXISTS save_history (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    display_name TEXT NOT NULL,
    score        INTEGER NOT NULL,
    created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_save_history_dn ON save_history(display_name, id DESC);

-- Ranking metadata. jti = NULL means the latest save was unauthenticated and
-- the row is excluded from /ranking. discord_id is derived via issued_tokens.
CREATE TABLE IF NOT EXISTS latest_saves (
    display_name TEXT PRIMARY KEY,
    score        INTEGER NOT NULL,
    history_id   INTEGER NOT NULL REFERENCES save_history(id),
    jti          TEXT REFERENCES issued_tokens(jti),
    updated_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_latest_saves_score ON latest_saves(score DESC);

CREATE TABLE IF NOT EXISTS challenge_ratelimit (
    display_name TEXT PRIMARY KEY,
    last_issued  INTEGER NOT NULL
);
