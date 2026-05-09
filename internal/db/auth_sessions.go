package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrAuthSessionNotFound = errors.New("auth session not found")
	ErrAuthSessionExpired  = errors.New("auth session expired")
)

type AuthSession struct {
	Token           string
	DiscordID       string
	DiscordUsername string
	ProposedName    string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

func (db *DB) InsertAuthSession(ctx context.Context, token, discordID, discordUsername, proposedName string, ttl time.Duration) error {
	now := db.clock.Now().UTC()
	_, err := db.ExecContext(ctx,
		`INSERT INTO auth_sessions (token, discord_id, discord_username, proposed_name, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		token, discordID, discordUsername, proposedName,
		now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339))
	return err
}

func (db *DB) GetAuthSession(ctx context.Context, token string) (*AuthSession, error) {
	row := db.QueryRowContext(ctx,
		`SELECT token, discord_id, discord_username, proposed_name, created_at, expires_at
		 FROM auth_sessions WHERE token = ?`, token)
	var s AuthSession
	var created, expires string
	if err := row.Scan(&s.Token, &s.DiscordID, &s.DiscordUsername, &s.ProposedName, &created, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAuthSessionNotFound
		}
		return nil, err
	}
	s.CreatedAt = parseTS(created)
	s.ExpiresAt = parseTS(expires)
	if db.clock.Now().After(s.ExpiresAt) {
		return nil, ErrAuthSessionExpired
	}
	return &s, nil
}

func (db *DB) ConsumeAuthSession(ctx context.Context, token string) (*AuthSession, error) {
	row := db.QueryRowContext(ctx,
		`DELETE FROM auth_sessions WHERE token = ?
		 RETURNING token, discord_id, discord_username, proposed_name, created_at, expires_at`, token)
	var s AuthSession
	var created, expires string
	if err := row.Scan(&s.Token, &s.DiscordID, &s.DiscordUsername, &s.ProposedName, &created, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAuthSessionNotFound
		}
		return nil, err
	}
	s.CreatedAt = parseTS(created)
	s.ExpiresAt = parseTS(expires)
	if db.clock.Now().After(s.ExpiresAt) {
		return nil, ErrAuthSessionExpired
	}
	return &s, nil
}

func (db *DB) DeleteExpiredAuthSessions(ctx context.Context) (int64, error) {
	res, err := db.ExecContext(ctx,
		`DELETE FROM auth_sessions WHERE expires_at < ?`, db.nowTS())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
