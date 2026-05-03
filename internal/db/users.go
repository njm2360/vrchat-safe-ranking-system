package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrDisplayNameTaken  = errors.New("display_name already bound to a different discord_id")
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
	var created, updated int64
	if err := row.Scan(&u.DiscordID, &u.DisplayName, &u.CurrentJTI, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.CreatedAt = time.Unix(created, 0)
	u.UpdatedAt = time.Unix(updated, 0)
	return &u, nil
}

// GetCurrentJWT returns the JWT string of the user's currently-active token,
// joining users.current_jti -> issued_tokens.jwt.
// Returns ErrUserNotFound if the user (or their current JWT) is missing.
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

// UpsertUserAndIssue records a newly-issued JWT in issued_tokens, blacklists
// any prior current_jti for this discord_id, and updates the user's
// display_name + current_jti pointer. All in one transaction.
//
// Returns an error if the displayName is currently bound to a different
// discord_id (one display_name belongs to at most one discord_id).
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

	now := db.nowUnix()

	// Record the new JWT in the registry first (FK targets must exist).
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO issued_tokens (jti, discord_id, display_name, jwt, issued_at)
		 VALUES (?, ?, ?, ?, ?)`,
		newJTI, discordID, displayName, newJWT, now); err != nil {
		return err
	}

	// Blacklist the old jti if there was one.
	if oldJTI.Valid && oldJTI.String != "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO jti_blacklist (jti, reason, created_at) VALUES (?, ?, ?)
			 ON CONFLICT(jti) DO NOTHING`,
			oldJTI.String, blacklistReason, now); err != nil {
			return err
		}
	}

	// Upsert the user, pointing current_jti at the new token.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (discord_id, display_name, current_jti, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(discord_id) DO UPDATE SET
		   display_name = excluded.display_name,
		   current_jti = excluded.current_jti,
		   updated_at = excluded.updated_at`,
		discordID, displayName, newJTI, now, now); err != nil {
		return err
	}

	return tx.Commit()
}

// ReleaseDisplayName forcibly releases a display_name binding (admin action,
// typically used when the name was hijacked before the legitimate VRChat owner
// could /register). It blacklists the holder's current JWT (so any saves they
// made drop out of /ranking) and deletes the users row so the legitimate owner
// can /register. Returns the discord_id that previously held the name, or
// ErrUserNotFound if no binding exists.
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
			jti.String, reason, db.nowUnix()); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM users WHERE display_name = ?`, displayName); err != nil {
		return "", err
	}
	return priorDiscordID, tx.Commit()
}

// Unregister blacklists the user's current JWT so they drop out of /ranking
// and can no longer /save. The users row is intentionally preserved so the
// display_name binding stays reserved against hijack by a different
// discord_id; the user can /register again later to mint a fresh token.
// Returns ErrUserNotFound if the discord_id is not registered.
func (db *DB) Unregister(ctx context.Context, discordID string) error {
	var jti sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT current_jti FROM users WHERE discord_id = ?`, discordID).Scan(&jti)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if !jti.Valid || jti.String == "" {
		return nil
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO jti_blacklist (jti, reason, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(jti) DO NOTHING`,
		jti.String, "self unregister", db.nowUnix())
	return err
}
