package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

var (
	ErrSaveNotFound  = errors.New("save not found")
	ErrDuplicateSave = errors.New("save with this generated_at already exists")
)

type SaveEntry struct {
	ID          int64
	DisplayName string
	Data        *savedata.Data
	CreatedAt   time.Time
}

func (db *DB) Save(ctx context.Context, displayName string, data *savedata.Data, jti string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := db.nowTS()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO save_history (display_name, score, generated_at, created_at) VALUES (?, ?, ?, ?)`,
		displayName, data.Score, data.GeneratedAt.UTC().Format(time.RFC3339), now)
	if err != nil {
		if strings.Contains(err.Error(), "save_history.generated_at") {
			return ErrDuplicateSave
		}
		return err
	}
	historyID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	var j sql.NullString
	if jti != "" {
		j = sql.NullString{String: jti, Valid: true}
	}

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO latest_saves (display_name, score, history_id, jti, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(display_name) DO UPDATE SET
		   score = excluded.score,
		   history_id = excluded.history_id,
		   jti = excluded.jti,
		   updated_at = excluded.updated_at`,
		displayName, data.Score, historyID, j, now); err != nil {
		return err
	}

	return tx.Commit()
}

func (db *DB) GetLatestSave(ctx context.Context, displayName string) (*SaveEntry, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, display_name, score, created_at FROM save_history
		 WHERE display_name = ?
		 ORDER BY id DESC
		 LIMIT 1`, displayName)
	return db.scanSaveEntry(row.Scan)
}

func (db *DB) scanSaveEntry(scan func(...any) error) (*SaveEntry, error) {
	var e SaveEntry
	var d savedata.Data
	var ts string
	if err := scan(&e.ID, &e.DisplayName, &d.Score, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSaveNotFound
		}
		return nil, err
	}
	e.Data = &d
	e.CreatedAt = parseTS(ts)
	return &e, nil
}

type RankingRow struct {
	Rank        int       `json:"rank"`
	DisplayName string    `json:"display_name"`
	Score       int64     `json:"score"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (db *DB) Ranking(ctx context.Context, limit int) ([]RankingRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx,
		`SELECT s.display_name, s.score, s.updated_at FROM latest_saves s
		 JOIN issued_tokens t ON t.jti = s.jti
		 LEFT JOIN jti_blacklist b ON b.jti = s.jti
		 LEFT JOIN discord_bans ban ON ban.discord_id = t.discord_id
		 LEFT JOIN display_name_bans dnb ON dnb.display_name = s.display_name
		 WHERE s.jti IS NOT NULL
		   AND b.jti IS NULL
		   AND ban.discord_id IS NULL
		   AND dnb.display_name IS NULL
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
		var updatedAt string
		if err := rows.Scan(&r.DisplayName, &r.Score, &updatedAt); err != nil {
			return nil, err
		}
		r.Rank = rank
		r.UpdatedAt = parseTS(updatedAt)
		out = append(out, r)
	}
	return out, rows.Err()
}
