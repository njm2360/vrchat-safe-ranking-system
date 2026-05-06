package oauth

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

type Fake struct {
	CallbackURL string
	CodeToUser  map[string]*User
	NextCode    string
	EchoCode    bool
	ExchangeErr error
}

func NewFake(callbackURL, defaultCode, discordID string) *Fake {
	return &Fake{
		CallbackURL: callbackURL,
		CodeToUser:  map[string]*User{defaultCode: {ID: discordID}},
		NextCode:    defaultCode,
	}
}

func NewFakeEcho() *Fake {
	return &Fake{EchoCode: true, CodeToUser: map[string]*User{}}
}

func (f *Fake) AuthURL(state string) string {
	code := f.NextCode
	if code == "" {
		for k := range f.CodeToUser {
			code = k
			break
		}
	}
	q := url.Values{}
	q.Set("code", code)
	q.Set("state", state)
	sep := "?"
	if u, err := url.Parse(f.CallbackURL); err == nil && u.RawQuery != "" {
		sep = "&"
	}
	return f.CallbackURL + sep + q.Encode()
}

func (f *Fake) Exchange(_ context.Context, code string) (*User, error) {
	if f.ExchangeErr != nil {
		return nil, f.ExchangeErr
	}
	if f.EchoCode {
		if code == "" {
			return nil, errors.New("oauth fake: empty code")
		}
		id, username, _ := strings.Cut(code, "|")
		if id == "" {
			return nil, errors.New("oauth fake: empty id in code")
		}
		return &User{ID: id, Username: username}, nil
	}
	if u, ok := f.CodeToUser[code]; ok {
		return u, nil
	}
	return nil, errors.New("oauth fake: unknown code")
}
