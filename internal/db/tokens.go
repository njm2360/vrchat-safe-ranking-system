package db

import (
	"context"
	"database/sql"
	"errors"
)

func (db *DB) IsJTIOwner(ctx context.Context, jti, displayName string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM users WHERE display_name = ? AND current_jti = ?`, displayName, jti).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) IsJTIBlacklisted(ctx context.Context, jti string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM jti_blacklist WHERE jti = ?`, jti).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) BlacklistJTI(ctx context.Context, jti, reason string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO jti_blacklist (jti, reason, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(jti) DO NOTHING`,
		jti, reason, db.nowTS())
	return err
}
