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
	APIAddr          string
	BaseURL          string
	DBPath           string
	JWTSecret        []byte
	HMACSaveSecret   []byte
	HMACLoadSecret   []byte
	TicketTTL        time.Duration
	TicketRetention  time.Duration
	ChallengeRateTTL time.Duration

	BotToken     string
	BotGuildID   string
	AdminUserIDs []string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	c := &Config{
		APIAddr:          getEnv("API_ADDR", ":8100"),
		BaseURL:          getEnv("BASE_URL", "http://localhost:8100"),
		DBPath:           getEnv("DB_PATH", "./data/vrc.db"),
		JWTSecret:        []byte(os.Getenv("JWT_SECRET")),
		HMACSaveSecret:   []byte(os.Getenv("HMAC_SAVE_SECRET")),
		HMACLoadSecret:   []byte(os.Getenv("HMAC_LOAD_SECRET")),
		TicketTTL:        getEnvDuration("TICKET_TTL", 5*time.Minute),
		TicketRetention:  getEnvDuration("TICKET_RETENTION", 24*time.Hour),
		ChallengeRateTTL: getEnvDuration("CHALLENGE_RATE_TTL", time.Minute),
		BotToken:         os.Getenv("BOT_TOKEN"),
		BotGuildID:       os.Getenv("BOT_GUILD_ID"),
		AdminUserIDs:     splitCSV(os.Getenv("ADMIN_USER_IDS")),
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

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (c *Config) IsAdmin(userID string) bool {
	for _, id := range c.AdminUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}
