package registration

import (
	"context"
	"errors"
	"fmt"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

type Store interface {
	IsDiscordIDBanned(ctx context.Context, discordID string) (bool, error)
	IsDisplayNameBanned(ctx context.Context, displayName string) (bool, error)
	GetUserByDiscordID(ctx context.Context, discordID string) (*db.User, error)
	UpsertUserAndIssue(ctx context.Context, discordID, displayName, jti, reason string) error
}

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
	DisplayName     string
	PrevDisplayName string
	IsRenewal       bool
}

var (
	ErrBanned            = errors.New("discord_id is banned")
	ErrDisplayNameBanned = errors.New("display_name is banned")
	ErrDisplayNameTaken  = errors.New("display_name already registered by another discord_id")
)

func (s *Service) Register(ctx context.Context, discordID, displayName string) (*Result, error) {
	banned, err := s.store.IsDiscordIDBanned(ctx, discordID)
	if err != nil {
		return nil, fmt.Errorf("ban check: %w", err)
	}
	if banned {
		return nil, ErrBanned
	}

	nameBanned, err := s.store.IsDisplayNameBanned(ctx, displayName)
	if err != nil {
		return nil, fmt.Errorf("display name ban check: %w", err)
	}
	if nameBanned {
		return nil, ErrDisplayNameBanned
	}

	existing, err := s.store.GetUserByDiscordID(ctx, discordID)
	isRenewal := err == nil && existing != nil && existing.CurrentJTI != ""
	if err != nil && !errors.Is(err, db.ErrUserNotFound) {
		return nil, err
	}
	prevDisplayName := ""
	if isRenewal {
		prevDisplayName = existing.DisplayName
	}

	jwt, jti, err := s.issuer.Issue(discordID, displayName)
	if err != nil {
		return nil, fmt.Errorf("issue jwt: %w", err)
	}

	if err := s.store.UpsertUserAndIssue(ctx, discordID, displayName, jti, "renewed via /auth/register"); err != nil {
		if errors.Is(err, db.ErrDisplayNameTaken) {
			return nil, ErrDisplayNameTaken
		}
		return nil, fmt.Errorf("upsert user: %w", err)
	}

	return &Result{
		JWT:             jwt,
		JTI:             jti,
		DisplayName:     displayName,
		PrevDisplayName: prevDisplayName,
		IsRenewal:       isRenewal,
	}, nil
}
