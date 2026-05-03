package db

import (
	"context"
	"database/sql"
	"errors"
)

func (db *DB) IsBanned(ctx context.Context, discordID string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM bans WHERE discord_id = ?`, discordID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) Ban(ctx context.Context, discordID, reason string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO bans (discord_id, reason, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(discord_id) DO UPDATE SET reason = excluded.reason`,
		discordID, reason, db.nowUnix())
	return err
}

func (db *DB) Unban(ctx context.Context, discordID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM bans WHERE discord_id = ?`, discordID)
	return err
}
