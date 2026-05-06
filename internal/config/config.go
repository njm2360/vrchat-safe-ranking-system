package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	APIAddr        string
	BaseURL        string
	DBPath         string
	JWTSecret      []byte
	HMACSaveSecret []byte
	HMACLoadSecret []byte
	OAuthStateTTL  time.Duration
	SessionTTL     time.Duration

	DiscordClientID     string
	DiscordClientSecret string
	OAuthRedirectURL    string

	OAuthMode string
}

const (
	OAuthModeDiscord = "discord"
	OAuthModeMock    = "mock"
)

func Load() (*Config, error) {
	_ = godotenv.Load()

	c := &Config{
		APIAddr:             getEnv("API_ADDR", ":8100"),
		BaseURL:             getEnv("BASE_URL", "http://localhost:8100"),
		DBPath:              getEnv("DB_PATH", "./data/vrc.db"),
		JWTSecret:           []byte(os.Getenv("JWT_SECRET")),
		HMACSaveSecret:      []byte(os.Getenv("HMAC_SAVE_SECRET")),
		HMACLoadSecret:      []byte(os.Getenv("HMAC_LOAD_SECRET")),
		OAuthStateTTL:       getEnvDuration("OAUTH_STATE_TTL", 5*time.Minute),
		SessionTTL:          getEnvDuration("SESSION_TTL", 30*time.Minute),
		DiscordClientID:     os.Getenv("DISCORD_CLIENT_ID"),
		DiscordClientSecret: os.Getenv("DISCORD_CLIENT_SECRET"),
		OAuthRedirectURL:    os.Getenv("OAUTH_REDIRECT_URL"),
		OAuthMode:           getEnv("OAUTH_MODE", OAuthModeDiscord),
	}

	if c.OAuthRedirectURL == "" {
		c.OAuthRedirectURL = strings.TrimRight(c.BaseURL, "/") + "/auth/callback"
	}

	if len(c.JWTSecret) < 16 {
		return nil, errors.New("JWT_SECRET must be set and at least 16 bytes")
	}
	if len(c.HMACSaveSecret) < 16 {
		return nil, errors.New("HMAC_SAVE_SECRET must be set and at least 16 bytes")
	}
	if len(c.HMACLoadSecret) < 16 {
		return nil, errors.New("HMAC_LOAD_SECRET must be set and at least 16 bytes")
	}
	switch c.OAuthMode {
	case OAuthModeDiscord:
		if c.DiscordClientID == "" {
			return nil, errors.New("DISCORD_CLIENT_ID must be set when OAUTH_MODE=discord")
		}
		if c.DiscordClientSecret == "" {
			return nil, errors.New("DISCORD_CLIENT_SECRET must be set when OAUTH_MODE=discord")
		}
	case OAuthModeMock:
		// mock mode: Discord credentials are not used.
	default:
		return nil, fmt.Errorf("OAUTH_MODE must be %q or %q (got %q)", OAuthModeDiscord, OAuthModeMock, c.OAuthMode)
	}

	return c, nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getEnvDuration(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	fmt.Fprintf(os.Stderr, "config: invalid duration for %s=%q, using default %s\n", k, v, def)
	return def
}
