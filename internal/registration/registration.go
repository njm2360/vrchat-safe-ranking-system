// Package registration encapsulates the /register flow used by both the
// Discord bot and the vrcsim e2e helper. It depends on small interfaces
// (Store, Issuer) so tests can substitute fakes.
package registration

import (
	"context"
	"errors"
	"fmt"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

// Store is the subset of *db.DB the registration flow consumes.
type Store interface {
	IsBanned(ctx context.Context, discordID string) (bool, error)
	ConsumeTicket(ctx context.Context, uuid string) (*db.Ticket, error)
	GetUserByDiscordID(ctx context.Context, discordID string) (*db.User, error)
	UpsertUserAndIssue(ctx context.Context, discordID, displayName, jti, jwt, reason string) error
}

// Issuer signs a fresh JWT for a (discordID, displayName) pair and returns
// both the signed string and its jti.
type Issuer interface {
	Issue(discordID, displayName string) (jwt string, jti string, err error)
}

type Service struct {
	store  Store
	issuer Issuer
}

func New(store Store, issuer Issuer) *Service {
	return &Service{store: store, issuer: issuer}
}

type Result struct {
	JWT             string
	JTI             string
	DisplayName     string // The DisplayName bound to the new JWT.
	PrevDisplayName string // The DisplayName the user had before this call (empty for new registrations).
	IsRenewal       bool
}

// Errors returned by Register. Callers should compare with errors.Is.
var (
	ErrTicketNotFound   = errors.New("ticket not found")
	ErrTicketExpired    = errors.New("ticket expired")
	ErrTicketUsed       = errors.New("ticket already used")
	ErrBanned           = errors.New("discord_id is banned")
	ErrDisplayNameTaken = errors.New("display_name already registered by another discord_id")
)

// Register consumes the ticket UUID and issues a JWT for (discordID,
// ticket.DisplayName). If discordID already had a JWT, its old jti is
// blacklisted as part of the same DB transaction. Banned discord_ids are
// rejected before the ticket is consumed.
func (s *Service) Register(ctx context.Context, discordID, ticketUUID string) (*Result, error) {
	banned, err := s.store.IsBanned(ctx, discordID)
	if err != nil {
		return nil, fmt.Errorf("ban check: %w", err)
	}
	if banned {
		return nil, ErrBanned
	}

	ticket, err := s.store.ConsumeTicket(ctx, ticketUUID)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrTicketNotFound):
			return nil, ErrTicketNotFound
		case errors.Is(err, db.ErrTicketExpired):
			return nil, ErrTicketExpired
		case errors.Is(err, db.ErrTicketUsed):
			return nil, ErrTicketUsed
		}
		return nil, fmt.Errorf("consume ticket: %w", err)
	}

	existing, err := s.store.GetUserByDiscordID(ctx, discordID)
	isRenewal := err == nil && existing != nil
	if err != nil && !errors.Is(err, db.ErrUserNotFound) {
		return nil, err
	}
	prevDisplayName := ""
	if isRenewal {
		prevDisplayName = existing.DisplayName
	}

	jwt, jti, err := s.issuer.Issue(discordID, ticket.DisplayName)
	if err != nil {
		return nil, fmt.Errorf("issue jwt: %w", err)
	}

	if err := s.store.UpsertUserAndIssue(ctx, discordID, ticket.DisplayName, jti, jwt, "renewed via /register"); err != nil {
		if errors.Is(err, db.ErrDisplayNameTaken) {
			return nil, ErrDisplayNameTaken
		}
		return nil, fmt.Errorf("upsert user: %w", err)
	}

	return &Result{
		JWT:             jwt,
		JTI:             jti,
		DisplayName:     ticket.DisplayName,
		PrevDisplayName: prevDisplayName,
		IsRenewal:       isRenewal,
	}, nil
}
