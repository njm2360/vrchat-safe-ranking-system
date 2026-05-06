package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
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

func (db *DB) GetCurrentJWT(ctx context.Context, discordID string) (jwt, displayName string, err error) {
	row := db.QueryRowContext(ctx,
		`SELECT t.jwt, t.display_name FROM users u
		 JOIN issued_tokens t ON t.jti = u.current_jti
		 WHERE u.discord_id = ?`, discordID)
	if err := row.Scan(&jwt, &displayName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrUserNotFound
		}
		return "", "", err
	}
	return jwt, displayName, nil
}

func (db *DB) UpsertUserAndIssue(ctx context.Context, discordID, displayName, newJTI, newJWT, blacklistReason string) error {
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

	// Read existing current_jti for this discord_id (if any)
	var oldJTI sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT current_jti FROM users WHERE discord_id = ?`, discordID).Scan(&oldJTI)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	now := db.nowTS()

	// Upsert the user first with current_jti = NULL so that the circular FK
	// (issued_tokens.discord_id → users) can be satisfied in the next step.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (discord_id, display_name, current_jti, created_at, updated_at)
		 VALUES (?, ?, NULL, ?, ?)
		 ON CONFLICT(discord_id) DO UPDATE SET
		   display_name = excluded.display_name,
		   current_jti  = NULL,
		   updated_at   = excluded.updated_at`,
		discordID, displayName, now, now); err != nil {
		return err
	}

	// Blacklist the old jti if there was one.
	// jti_blacklist has no FK to issued_tokens, so this is always safe.
	if oldJTI.Valid && oldJTI.String != "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO jti_blacklist (jti, reason, created_at) VALUES (?, ?, ?)
			 ON CONFLICT(jti) DO NOTHING`,
			oldJTI.String, blacklistReason, now); err != nil {
			return err
		}
	}

	// Record the new JWT; users row already exists so discord_id FK is satisfied.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO issued_tokens (jti, discord_id, display_name, jwt, issued_at)
		 VALUES (?, ?, ?, ?, ?)`,
		newJTI, discordID, displayName, newJWT, now); err != nil {
		return err
	}

	// Point current_jti at the new token now that issued_tokens row exists.
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET current_jti = ?, updated_at = ? WHERE discord_id = ?`,
		newJTI, now, discordID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE latest_saves SET jti = ?
		 WHERE display_name = ?
		   AND jti IN (SELECT jti FROM issued_tokens WHERE discord_id = ?)`,
		newJTI, displayName, discordID); err != nil {
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
	// Clear current_jti before DELETE to avoid ambiguity in the circular FK
	// (users.current_jti → issued_tokens) when CASCADE fires.
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET current_jti = NULL WHERE display_name = ?`, displayName); err != nil {
		return "", err
	}
	// CASCADE deletes all issued_tokens for this discord_id.
	// latest_saves.jti is SET NULL automatically, removing the entry from ranking.
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
