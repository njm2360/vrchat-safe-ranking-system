package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrOAuthStateNotFound = errors.New("oauth state not found")
	ErrOAuthStateExpired  = errors.New("oauth state expired")
)

type OAuthState struct {
	State        string
	ProposedName string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

func (db *DB) InsertOAuthState(ctx context.Context, state, proposedName string, ttl time.Duration) error {
	now := db.clock.Now().UTC()
	_, err := db.ExecContext(ctx,
		`INSERT INTO oauth_states (state, proposed_name, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		state, proposedName,
		now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339))
	return err
}

func (db *DB) ConsumeOAuthState(ctx context.Context, state string) (*OAuthState, error) {
	row := db.QueryRowContext(ctx,
		`DELETE FROM oauth_states WHERE state = ?
		 RETURNING state, proposed_name, created_at, expires_at`, state)
	var s OAuthState
	var created, expires string
	if err := row.Scan(&s.State, &s.ProposedName, &created, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOAuthStateNotFound
		}
		return nil, err
	}
	s.CreatedAt = parseTS(created)
	s.ExpiresAt = parseTS(expires)
	if db.clock.Now().After(s.ExpiresAt) {
		return nil, ErrOAuthStateExpired
	}
	return &s, nil
}

func (db *DB) DeleteExpiredOAuthStates(ctx context.Context) (int64, error) {
	res, err := db.ExecContext(ctx,
		`DELETE FROM oauth_states WHERE expires_at < ?`, db.nowTS())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
