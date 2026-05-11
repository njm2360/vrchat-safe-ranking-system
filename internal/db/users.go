package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrDisplayNameTaken = errors.New("display_name already bound to a different discord_id")
)

type User struct {
	DiscordID   string
	DisplayName string
	CurrentJTI  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (db *DB) GetUserByDiscordID(ctx context.Context, discordID string) (*User, error) {
	return db.scanUser(db.QueryRowContext(ctx,
		`SELECT discord_id, display_name, COALESCE(current_jti, ''), created_at, updated_at
		 FROM users WHERE discord_id = ?`, discordID))
}

func (db *DB) IsDisplayNameRegistered(ctx context.Context, displayName string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM users WHERE display_name = ?`, displayName).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) GetUserByDisplayName(ctx context.Context, displayName string) (*User, error) {
	return db.scanUser(db.QueryRowContext(ctx,
		`SELECT discord_id, display_name, COALESCE(current_jti, ''), created_at, updated_at
		 FROM users WHERE display_name = ?`, displayName))
}

func (db *DB) scanUser(row *sql.Row) (*User, error) {
	var u User
	var created, updated string
	if err := row.Scan(&u.DiscordID, &u.DisplayName, &u.CurrentJTI, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.CreatedAt = parseTS(created)
	u.UpdatedAt = parseTS(updated)
	return &u, nil
}

func (db *DB) UpsertUserAndIssue(ctx context.Context, discordID, displayName, newJTI, blacklistReason string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Reject if displayName is already bound to a different discord_id
	var existingDiscord string
	err = tx.QueryRowContext(ctx,
		`SELECT discord_id FROM users WHERE display_name = ?`, displayName).Scan(&existingDiscord)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && existingDiscord != discordID {
		return ErrDisplayNameTaken
	}

	// Snapshot the current state before any mutation.
	var oldJTI sql.NullString
	var oldDisplayName string
	err = tx.QueryRowContext(ctx,
		`SELECT current_jti, display_name FROM users WHERE discord_id = ?`, discordID).Scan(&oldJTI, &oldDisplayName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	now := db.nowTS()

	// Upsert the user row with current_jti = NULL first; issued_tokens will be
	// inserted next and current_jti pointed at it afterwards.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (discord_id, display_name, current_jti, created_at, updated_at)
		 VALUES (?, ?, NULL, ?, ?)
		 ON CONFLICT(discord_id) DO UPDATE SET
		   display_name = excluded.display_name,
		   current_jti  = NULL,
		   updated_at   = excluded.updated_at`,
		discordID, displayName, now, now); err != nil {
		var sqliteErr *sqlite.Error
		if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return ErrDisplayNameTaken
		}
		return err
	}

	// Invalidate the previous token so it can no longer be used.
	if oldJTI.Valid && oldJTI.String != "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO jti_blacklist (jti, reason, created_at) VALUES (?, ?, ?)
			 ON CONFLICT(jti) DO NOTHING`,
			oldJTI.String, blacklistReason, now); err != nil {
			return err
		}
	}

	// On rename, drop the stale latest_saves row for the old display name.
	if oldDisplayName != "" && oldDisplayName != displayName {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM latest_saves WHERE display_name = ?`, oldDisplayName); err != nil {
			return err
		}
	}

	// Record the new token as an immutable audit entry.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO issued_tokens (jti, discord_id, display_name, issued_at)
		 VALUES (?, ?, ?, ?)`,
		newJTI, discordID, displayName, now); err != nil {
		return err
	}

	// Activate the new token.
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET current_jti = ?, updated_at = ? WHERE discord_id = ?`,
		newJTI, now, discordID); err != nil {
		return err
	}

	return tx.Commit()
}

func (db *DB) ReleaseDisplayName(ctx context.Context, displayName, reason string) (priorDiscordID string, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var jti sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT discord_id, current_jti FROM users WHERE display_name = ?`, displayName).Scan(&priorDiscordID, &jti)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", err
	}
	if jti.Valid && jti.String != "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO jti_blacklist (jti, reason, created_at) VALUES (?, ?, ?)
			 ON CONFLICT(jti) DO NOTHING`,
			jti.String, reason, db.nowTS()); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM users WHERE display_name = ?`, displayName); err != nil {
		return "", err
	}
	return priorDiscordID, tx.Commit()
}

func (db *DB) Unregister(ctx context.Context, discordID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var jti sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT current_jti FROM users WHERE discord_id = ?`, discordID).Scan(&jti)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if jti.Valid && jti.String != "" {
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO jti_blacklist (jti, reason, created_at) VALUES (?, ?, ?)
			 ON CONFLICT(jti) DO NOTHING`,
			jti.String, "self unregister", db.nowTS()); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE users SET current_jti = NULL, updated_at = ? WHERE discord_id = ?`,
		db.nowTS(), discordID); err != nil {
		return err
	}
	return tx.Commit()
}
