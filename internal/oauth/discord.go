package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Discord endpoints. Overridable via DiscordConfig for tests.
const (
	defaultDiscordAuthorizeURL = "https://discord.com/api/oauth2/authorize"
	defaultDiscordTokenURL     = "https://discord.com/api/oauth2/token"
	defaultDiscordUserURL      = "https://discord.com/api/users/@me"
)

// DiscordConfig wires the Discord OAuth2 application into a Provider.
type DiscordConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string

	// Optional overrides for tests / self-hosted Discord-compatible
	// servers. Empty values fall back to discord.com defaults.
	AuthorizeURL string
	TokenURL     string
	UserURL      string
	HTTPClient   *http.Client
}

type DiscordProvider struct {
	cfg          DiscordConfig
	authorizeURL string
	tokenURL     string
	userURL      string
	http         *http.Client
}

func NewDiscord(cfg DiscordConfig) *DiscordProvider {
	p := &DiscordProvider{
		cfg:          cfg,
		authorizeURL: cfg.AuthorizeURL,
		tokenURL:     cfg.TokenURL,
		userURL:      cfg.UserURL,
		http:         cfg.HTTPClient,
	}
	if p.authorizeURL == "" {
		p.authorizeURL = defaultDiscordAuthorizeURL
	}
	if p.tokenURL == "" {
		p.tokenURL = defaultDiscordTokenURL
	}
	if p.userURL == "" {
		p.userURL = defaultDiscordUserURL
	}
	if p.http == nil {
		p.http = http.DefaultClient
	}
	return p
}

func (p *DiscordProvider) AuthURL(state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", p.cfg.ClientID)
	q.Set("redirect_uri", p.cfg.RedirectURL)
	q.Set("scope", "identify")
	q.Set("state", state)
	return p.authorizeURL + "?" + q.Encode()
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

type userResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

func (p *DiscordProvider) Exchange(ctx context.Context, code string) (*User, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", p.cfg.RedirectURL)
	form.Set("client_id", p.cfg.ClientID)
	form.Set("client_secret", p.cfg.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errBody struct {
			Description string `json:"error_description"`
		}
		if json.Unmarshal(body, &errBody) == nil &&
			strings.Contains(strings.ToLower(errBody.Description), "rate limit") {
			return nil, ErrRateLimited
		}
		return nil, fmt.Errorf("token endpoint: status %d: %s", resp.StatusCode, string(body))
	}
	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint: empty access_token")
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userURL, nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	userResp, err := p.http.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("user endpoint: %w", err)
	}
	defer userResp.Body.Close()
	if userResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(userResp.Body)
		return nil, fmt.Errorf("user endpoint: status %d: %s", userResp.StatusCode, string(body))
	}
	var u userResponse
	if err := json.NewDecoder(userResp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("user endpoint: empty id")
	}
	return &User{ID: u.ID, Username: u.Username}, nil
}
