package oauth

import (
	"context"
	"errors"
)

// ErrRateLimited is returned by Exchange when the OAuth provider responds with
// a rate-limit error (e.g. Discord's "too many tokens" 400 response).
var ErrRateLimited = errors.New("oauth: rate limited by provider")

type User struct {
	ID       string
	Username string
}

type Provider interface {
	AuthURL(state string) string

	Exchange(ctx context.Context, code string) (*User, error)
}
