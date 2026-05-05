// Package vrcclient mimics what a VRChat Udon client would do: build URLs,
// compute HMAC signatures, and issue GET requests against the ranking API.
//
// All operations are GET-only (matching the Udon constraint).
package vrcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

type Client struct {
	BaseURL    string
	HTTP       *http.Client
	SaveSecret []byte
	LoadSecret []byte
}

func New(baseURL string, saveSecret, loadSecret []byte) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTP:       http.DefaultClient,
		SaveSecret: saveSecret,
		LoadSecret: loadSecret,
	}
}

func (c *Client) ChallengeURL(displayName string) string {
	q := url.Values{}
	q.Set("name", displayName)
	return c.BaseURL + "/challenge?" + q.Encode()
}

func (c *Client) RequestChallenge(ctx context.Context, displayName string) (string, error) {
	body, status, err := c.get(ctx, c.ChallengeURL(displayName))
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("challenge: status %d: %s", status, body)
	}
	return strings.TrimSpace(body), nil
}

type SaveParams struct {
	Score int64
	JWT   string
}

func (c *Client) SaveURL(p SaveParams) string {
	sig := auth.SignHex(c.SaveSecret, auth.SaveSigMessage(p.Score))
	q := url.Values{}
	q.Set("score", strconv.FormatInt(p.Score, 10))
	q.Set("jwt", p.JWT)
	q.Set("sig", sig)
	return c.BaseURL + "/save?" + q.Encode()
}

func (c *Client) Save(ctx context.Context, p SaveParams) (string, error) {
	body, status, err := c.get(ctx, c.SaveURL(p))
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("save: status %d: %s", status, body)
	}
	return strings.TrimSpace(body), nil
}

type LoadParams struct {
	JWT string
}

func (c *Client) LoadURL(p LoadParams) string {
	q := url.Values{}
	q.Set("jwt", p.JWT)
	return c.BaseURL + "/load?" + q.Encode()
}

// Load returns the score string. Returns ("", nil) when there is no save yet.
func (c *Client) Load(ctx context.Context, p LoadParams) (string, error) {
	body, status, err := c.get(ctx, c.LoadURL(p))
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", nil
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("load: status %d: %s", status, body)
	}

	var resp struct {
		Score int64  `json:"score"`
		Sig   string `json:"sig"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return "", fmt.Errorf("load: invalid response: %w", err)
	}
	if !auth.VerifyHex(c.LoadSecret, auth.LoadSigMessage(resp.Score), resp.Sig) {
		return "", fmt.Errorf("load: response sig invalid (MITM?)")
	}
	return strconv.FormatInt(resp.Score, 10), nil
}

func (c *Client) get(ctx context.Context, u string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(b), resp.StatusCode, nil
}
