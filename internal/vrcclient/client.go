package vrcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

type Client struct {
	BaseURL    string
	HTTP       *http.Client
	SaveSecret []byte
	LoadSecret []byte
	AuthSecret []byte
}

func New(baseURL string, saveSecret, loadSecret, authSecret []byte) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTP:       http.DefaultClient,
		SaveSecret: saveSecret,
		LoadSecret: loadSecret,
		AuthSecret: authSecret,
	}
}

type SaveParams struct {
	Data        *savedata.Data
	JWT         string
	DisplayName string
}

func (c *Client) SaveURL(p SaveParams) (string, error) {
	if p.Data == nil {
		return "", fmt.Errorf("vrcclient: SaveParams.Data is nil")
	}
	body, err := savedata.Marshal(p.Data)
	if err != nil {
		return "", fmt.Errorf("marshal save data: %w", err)
	}
	sig := auth.SignHex(c.SaveSecret, body, []byte(p.DisplayName))
	q := url.Values{}
	q.Set("data", string(body))
	q.Set("display_name", p.DisplayName)
	q.Set("jwt", p.JWT)
	q.Set("sig", sig)
	return c.BaseURL + "/save?" + q.Encode(), nil
}

func (c *Client) Save(ctx context.Context, p SaveParams) (string, error) {
	u, err := c.SaveURL(p)
	if err != nil {
		return "", err
	}
	body, status, err := c.get(ctx, u)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("save: status %d: %s", status, body)
	}
	return strings.TrimSpace(body), nil
}

type LoadParams struct {
	JWT         string
	DisplayName string
}

type AuthStartParams struct {
	DisplayName   string
	FakeDiscordID string
	FakeUsername  string
}

func (c *Client) AuthStartURL(p AuthStartParams) string {
	sig := auth.SignHex(c.AuthSecret, []byte(p.DisplayName))
	q := url.Values{}
	q.Set("display_name", p.DisplayName)
	q.Set("sig", sig)
	if p.FakeDiscordID != "" {
		q.Set("fake_discord_id", p.FakeDiscordID)
	}
	if p.FakeUsername != "" {
		q.Set("fake_username", p.FakeUsername)
	}
	return c.BaseURL + "/auth/start?" + q.Encode()
}

func (c *Client) LoadURL(p LoadParams) string {
	sig := auth.SignHex(c.LoadSecret, []byte(p.DisplayName))
	q := url.Values{}
	q.Set("display_name", p.DisplayName)
	q.Set("sig", sig)
	q.Set("jwt", p.JWT)
	return c.BaseURL + "/load?" + q.Encode()
}

func (c *Client) Load(ctx context.Context, p LoadParams) (*savedata.Data, error) {
	body, status, err := c.get(ctx, c.LoadURL(p))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("load: status %d: %s", status, body)
	}

	var resp struct {
		Data json.RawMessage `json:"data"`
		Sig  string          `json:"sig"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, fmt.Errorf("load: invalid response: %w", err)
	}
	if len(resp.Data) == 0 || resp.Sig == "" {
		return nil, fmt.Errorf("load: response missing data or sig")
	}
	if !auth.VerifyHex(c.LoadSecret, resp.Sig, resp.Data) {
		return nil, fmt.Errorf("load: response sig invalid (MITM?)")
	}
	d, err := savedata.Unmarshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("load: parse data: %w", err)
	}
	return d, nil
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
