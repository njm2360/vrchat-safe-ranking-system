package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/idgen"
)

const testSecret = "test-secret-32-bytes-padded-padding"

func TestJWTRoundtrip(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
	ig := idgen.NewSequential("jti")
	issuer := auth.NewJWTIssuer([]byte(testSecret), auth.WithClock(fc), auth.WithIDGen(ig))

	tok, jti, err := issuer.Issue("dscord-1", "alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if jti != "jti-0001" {
		t.Errorf("jti = %q, want jti-0001", jti)
	}
	if !strings.HasPrefix(tok, "eyJ") {
		t.Errorf("token does not look like a JWT: %q", tok)
	}

	claims, err := issuer.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.DiscordID != "dscord-1" || claims.DisplayName != "alice" || claims.JTI != "jti-0001" {
		t.Errorf("claims mismatch: %+v", claims)
	}
	if claims.IssuedAt != fc.Now().Unix() {
		t.Errorf("iat = %d, want %d", claims.IssuedAt, fc.Now().Unix())
	}
}

func TestJWTVerifyTamperedSignature(t *testing.T) {
	issuer := auth.NewJWTIssuer([]byte(testSecret))
	tok, _, err := issuer.Issue("d", "alice")
	if err != nil {
		t.Fatal(err)
	}
	// Flip last char of signature
	tampered := tok[:len(tok)-1] + "X"
	if tampered == tok {
		tampered = tok[:len(tok)-1] + "Y"
	}
	if _, err := issuer.Verify(tampered); err == nil {
		t.Fatal("Verify accepted a tampered signature")
	}
}

func TestJWTVerifyWrongKey(t *testing.T) {
	a := auth.NewJWTIssuer([]byte(testSecret))
	b := auth.NewJWTIssuer([]byte("different-key-also-32-bytes-min-pad"))

	tok, _, err := a.Issue("d", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Verify(tok); err == nil {
		t.Fatal("Verify accepted token signed with a different key")
	}
}

// alg=none attack: an attacker forges a token with the unsecured alg, hoping
// our verifier will accept it. Verify must reject anything that isn't HMAC.
func TestJWTRejectsAlgNone(t *testing.T) {
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, auth.Claims{
		DiscordID: "d", DisplayName: "alice", JTI: "x", IssuedAt: 1,
	})
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	issuer := auth.NewJWTIssuer([]byte(testSecret))
	if _, err := issuer.Verify(signed); err == nil {
		t.Fatal("Verify accepted an alg=none token")
	}
}

func TestJWTRejectsMissingClaims(t *testing.T) {
	cases := []struct {
		name   string
		claims auth.Claims
	}{
		{"no jti", auth.Claims{DiscordID: "d", DisplayName: "alice", IssuedAt: 1}},
		{"no discord_id", auth.Claims{DisplayName: "alice", IssuedAt: 1, JTI: "x"}},
		{"no display_name", auth.Claims{DiscordID: "d", IssuedAt: 1, JTI: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, tc.claims)
			signed, err := tok.SignedString([]byte(testSecret))
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			issuer := auth.NewJWTIssuer([]byte(testSecret))
			if _, err := issuer.Verify(signed); err == nil {
				t.Errorf("Verify accepted token with missing claim (%s)", tc.name)
			}
		})
	}
}

func TestJWTSequentialIssue(t *testing.T) {
	ig := idgen.NewSequential("jti")
	issuer := auth.NewJWTIssuer([]byte(testSecret), auth.WithIDGen(ig))

	_, j1, _ := issuer.Issue("d", "alice")
	_, j2, _ := issuer.Issue("d", "alice")
	if j1 == j2 {
		t.Errorf("expected unique jtis, got %q twice", j1)
	}
	if j1 != "jti-0001" || j2 != "jti-0002" {
		t.Errorf("unexpected jtis: %q, %q", j1, j2)
	}
}
