package db

import (
	"context"
	"database/sql"
	"errors"
)

func (db *DB) IsDiscordIDBanned(ctx context.Context, discordID string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM discord_bans WHERE discord_id = ?`, discordID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) BanDiscordID(ctx context.Context, discordID, reason string) error {
	now := db.nowTS()
	_, err := db.ExecContext(ctx,
		`INSERT INTO discord_bans (discord_id, reason, created_at, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(discord_id) DO UPDATE SET reason = excluded.reason, updated_at = excluded.updated_at`,
		discordID, reason, now, now)
	return err
}

func (db *DB) UnbanDiscordID(ctx context.Context, discordID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM discord_bans WHERE discord_id = ?`, discordID)
	return err
}

func (db *DB) IsDisplayNameBanned(ctx context.Context, displayName string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM display_name_bans WHERE display_name = ?`, displayName).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) BanDisplayName(ctx context.Context, displayName, reason string) error {
	now := db.nowTS()
	_, err := db.ExecContext(ctx,
		`INSERT INTO display_name_bans (display_name, reason, created_at, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(display_name) DO UPDATE SET reason = excluded.reason, updated_at = excluded.updated_at`,
		displayName, reason, now, now)
	return err
}

func (db *DB) UnbanDisplayName(ctx context.Context, displayName string) error {
	_, err := db.ExecContext(ctx,
		`DELETE FROM display_name_bans WHERE display_name = ?`, displayName)
	return err
}
