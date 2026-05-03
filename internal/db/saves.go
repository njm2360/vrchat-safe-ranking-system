package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrSaveNotFound = errors.New("save not found")

type SaveEntry struct {
	ID          int64
	DisplayName string
	Score       int64
	CreatedAt   time.Time
}

// Save appends a save_history row and updates the latest_saves row in a single
// transaction. jti is empty when the save was unauthenticated (no/invalid JWT);
// the latest_saves row is then excluded from ranking via IS NOT NULL.
// The discord_id is not stored here — it is derived from issued_tokens via jti.
func (db *DB) Save(ctx context.Context, displayName string, score int64, jti string) (historyID int64, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := db.nowUnix()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO save_history (display_name, score, created_at) VALUES (?, ?, ?)`,
		displayName, score, now)
	if err != nil {
		return 0, err
	}
	historyID, err = res.LastInsertId()
	if err != nil {
		return 0, err
	}

	var j sql.NullString
	if jti != "" {
		j = sql.NullString{String: jti, Valid: true}
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO latest_saves (display_name, score, history_id, jti, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(display_name) DO UPDATE SET
		   score = excluded.score,
		   history_id = excluded.history_id,
		   jti = excluded.jti,
		   updated_at = excluded.updated_at`,
		displayName, score, historyID, j, now)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return historyID, nil
}

// GetLatestSave returns the most recent save_history row for displayName.
// Reads from the append-only history (the source of truth) rather than the
// latest_saves table — latest_saves carries ranking metadata only.
// Returns ErrSaveNotFound if no save exists.
func (db *DB) GetLatestSave(ctx context.Context, displayName string) (*SaveEntry, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, display_name, score, created_at FROM save_history
		 WHERE display_name = ?
		 ORDER BY id DESC
		 LIMIT 1`, displayName)
	var e SaveEntry
	var ts int64
	if err := row.Scan(&e.ID, &e.DisplayName, &e.Score, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSaveNotFound
		}
		return nil, err
	}
	e.CreatedAt = time.Unix(ts, 0)
	return &e, nil
}

// GetSaveHistory returns the most recent save_history rows for displayName.
func (db *DB) GetSaveHistory(ctx context.Context, displayName string, limit int) ([]SaveEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, display_name, score, created_at FROM save_history
		 WHERE display_name = ?
		 ORDER BY id DESC
		 LIMIT ?`, displayName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SaveEntry
	for rows.Next() {
		var e SaveEntry
		var ts int64
		if err := rows.Scan(&e.ID, &e.DisplayName, &e.Score, &ts); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

type RankingRow struct {
	Rank        int    `json:"rank"`
	DisplayName string `json:"display_name"`
	Score       int64  `json:"score"`
	UpdatedAt   int64  `json:"updated_at"`
}

// Ranking returns the leaderboard, excluding rows whose latest save lacked a
// JWT, whose JTI is blacklisted, or whose Discord ID is banned.
func (db *DB) Ranking(ctx context.Context, limit int) ([]RankingRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx,
		`SELECT s.display_name, s.score, s.updated_at FROM latest_saves s
		 JOIN issued_tokens t ON t.jti = s.jti
		 LEFT JOIN jti_blacklist b ON b.jti = s.jti
		 LEFT JOIN bans ban ON ban.discord_id = t.discord_id
		 WHERE s.jti IS NOT NULL
		   AND b.jti IS NULL
		   AND ban.discord_id IS NULL
		 ORDER BY s.score DESC, s.updated_at ASC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RankingRow
	rank := 0
	for rows.Next() {
		rank++
		var r RankingRow
		if err := rows.Scan(&r.DisplayName, &r.Score, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Rank = rank
		out = append(out, r)
	}
	return out, rows.Err()
}
