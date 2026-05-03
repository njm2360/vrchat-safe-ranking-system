package auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"

	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/idgen"
)

type Claims struct {
	DiscordID   string `json:"discord_id"`
	DisplayName string `json:"display_name"`
	IssuedAt    int64  `json:"iat"`
	JTI         string `json:"jti"`
	jwt.RegisteredClaims
}

type JWTIssuer struct {
	secret []byte
	idgen  idgen.Generator
	clock  clock.Clock
}

type JWTOption func(*JWTIssuer)

func WithIDGen(g idgen.Generator) JWTOption { return func(j *JWTIssuer) { j.idgen = g } }
func WithClock(c clock.Clock) JWTOption     { return func(j *JWTIssuer) { j.clock = c } }

func NewJWTIssuer(secret []byte, opts ...JWTOption) *JWTIssuer {
	j := &JWTIssuer{secret: secret, idgen: idgen.Real{}, clock: clock.System{}}
	for _, opt := range opts {
		opt(j)
	}
	return j
}

// Issue creates a new JWT for the given identity. Returns the token string and jti.
func (j *JWTIssuer) Issue(discordID, displayName string) (token string, jti string, err error) {
	jti = j.idgen.NewUUID()
	claims := Claims{
		DiscordID:   discordID,
		DisplayName: displayName,
		IssuedAt:    j.clock.Now().Unix(),
		JTI:         jti,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(j.secret)
	if err != nil {
		return "", "", err
	}
	return signed, jti, nil
}

func (j *JWTIssuer) Verify(tokenStr string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.JTI == "" || claims.DiscordID == "" || claims.DisplayName == "" {
		return nil, errors.New("missing required claims")
	}
	return claims, nil
}
