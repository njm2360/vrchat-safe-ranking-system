package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrTicketNotFound = errors.New("ticket not found")
	ErrTicketExpired  = errors.New("ticket expired")
	ErrTicketUsed     = errors.New("ticket already used")
)

type Ticket struct {
	UUID        string
	DisplayName string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
}

func (db *DB) InsertTicket(ctx context.Context, uuid, displayName string, ttl time.Duration) error {
	now := db.clock.Now()
	_, err := db.ExecContext(ctx,
		`INSERT INTO tickets (uuid, display_name, issued_at, expires_at) VALUES (?, ?, ?, ?)`,
		uuid, displayName, now.Unix(), now.Add(ttl).Unix())
	return err
}

func (db *DB) GetTicket(ctx context.Context, uuid string) (*Ticket, error) {
	row := db.QueryRowContext(ctx,
		`SELECT uuid, display_name, issued_at, expires_at, consumed_at FROM tickets WHERE uuid = ?`, uuid)
	var t Ticket
	var issued, expires int64
	var consumed sql.NullInt64
	if err := row.Scan(&t.UUID, &t.DisplayName, &issued, &expires, &consumed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTicketNotFound
		}
		return nil, err
	}
	t.IssuedAt = time.Unix(issued, 0)
	t.ExpiresAt = time.Unix(expires, 0)
	if consumed.Valid {
		c := time.Unix(consumed.Int64, 0)
		t.ConsumedAt = &c
	}
	return &t, nil
}

// ConsumeTicket atomically marks a ticket as consumed, returning the ticket on success.
// Returns ErrTicketNotFound, ErrTicketExpired, or ErrTicketUsed.
func (db *DB) ConsumeTicket(ctx context.Context, uuid string) (*Ticket, error) {
	now := db.nowUnix()
	res, err := db.ExecContext(ctx,
		`UPDATE tickets SET consumed_at = ?
		 WHERE uuid = ? AND consumed_at IS NULL AND expires_at > ?`,
		now, uuid, now)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		// Investigate why
		t, err := db.GetTicket(ctx, uuid)
		if err != nil {
			return nil, err
		}
		if t.ConsumedAt != nil {
			return nil, ErrTicketUsed
		}
		return nil, ErrTicketExpired
	}
	return db.GetTicket(ctx, uuid)
}

func (db *DB) DeleteExpiredTickets(ctx context.Context) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM tickets WHERE expires_at < ?`, db.nowUnix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CheckChallengeRate returns whether a new challenge can be issued for
// displayName. If a previous issue happened within window, allowed=false and
// last is the time of that issue.
func (db *DB) CheckChallengeRate(ctx context.Context, displayName string, window time.Duration) (last time.Time, allowed bool, err error) {
	row := db.QueryRowContext(ctx, `SELECT last_issued FROM challenge_ratelimit WHERE display_name = ?`, displayName)
	var ts int64
	err = row.Scan(&ts)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, true, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	last = time.Unix(ts, 0)
	if db.clock.Now().Sub(last) < window {
		return last, false, nil
	}
	return last, true, nil
}

func (db *DB) UpsertChallengeRate(ctx context.Context, displayName string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO challenge_ratelimit (display_name, last_issued) VALUES (?, ?)
		 ON CONFLICT(display_name) DO UPDATE SET last_issued = excluded.last_issued`,
		displayName, db.nowUnix())
	return err
}
