package api_test

import (
	"context"
	"errors"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

// fakeSaveStore implements api.SaveStore.
type fakeSaveStore struct {
	saveCalls  []saveCall
	saveErr    error
	latestRet  *db.SaveEntry
	latestErr  error
	rankingRet []db.RankingRow
	rankingErr error
}

type saveCall struct {
	DisplayName string
	Data        *savedata.Data
	JTI         string
}

func (f *fakeSaveStore) Save(_ context.Context, dn string, data *savedata.Data, jti string) error {
	f.saveCalls = append(f.saveCalls, saveCall{dn, data, jti})
	return f.saveErr
}
func (f *fakeSaveStore) GetLatestSave(_ context.Context, _ string) (*db.SaveEntry, error) {
	return f.latestRet, f.latestErr
}
func (f *fakeSaveStore) Ranking(_ context.Context, _ int) ([]db.RankingRow, error) {
	return f.rankingRet, f.rankingErr
}

// fakeAuthStore implements api.AuthStore.
type fakeAuthStore struct {
	jtiBlacklisted  bool
	jtiBlacklistErr error
	dnBanned        bool
	dnBannedErr     error
	insertCalls          []insertStateCall
	insertErr            error
	consumeRet           *db.OAuthState
	consumeErr           error
	banned               bool
	bannedErr            error
	currentJWT           string
	currentDN            string
	currentJWTErr        error
	userByDiscordRet     *db.User
	userByDiscordErr     error
	userByDisplayNameRet *db.User
	userByDisplayNameErr error
	unregisterErr        error
	unregisterCalls      int
	sessionInsertCalls   []insertSessionCall
	sessionInsertErr     error
	sessionGetRet        *db.AuthSession
	sessionGetErr        error
	sessionConsumeRet    *db.AuthSession
	sessionConsumeErr    error
}

type insertStateCall struct {
	State        string
	ProposedName string
	TTL          time.Duration
}

type insertSessionCall struct {
	Token           string
	DiscordID       string
	DiscordUsername string
	ProposedName    string
	TTL             time.Duration
}

func (f *fakeAuthStore) IsJTIBlacklisted(_ context.Context, _ string) (bool, error) {
	return f.jtiBlacklisted, f.jtiBlacklistErr
}
func (f *fakeAuthStore) IsDisplayNameBanned(_ context.Context, _ string) (bool, error) {
	return f.dnBanned, f.dnBannedErr
}
func (f *fakeAuthStore) InsertOAuthState(_ context.Context, state, name string, ttl time.Duration) error {
	f.insertCalls = append(f.insertCalls, insertStateCall{state, name, ttl})
	return f.insertErr
}
func (f *fakeAuthStore) ConsumeOAuthState(_ context.Context, _ string) (*db.OAuthState, error) {
	return f.consumeRet, f.consumeErr
}
func (f *fakeAuthStore) IsDiscordIDBanned(_ context.Context, _ string) (bool, error) {
	return f.banned, f.bannedErr
}
func (f *fakeAuthStore) GetCurrentJWT(_ context.Context, _ string) (string, string, error) {
	return f.currentJWT, f.currentDN, f.currentJWTErr
}
func (f *fakeAuthStore) GetUserByDiscordID(_ context.Context, _ string) (*db.User, error) {
	return f.userByDiscordRet, f.userByDiscordErr
}
func (f *fakeAuthStore) GetUserByDisplayName(_ context.Context, _ string) (*db.User, error) {
	return f.userByDisplayNameRet, f.userByDisplayNameErr
}
func (f *fakeAuthStore) Unregister(_ context.Context, _ string) error {
	f.unregisterCalls++
	return f.unregisterErr
}
func (f *fakeAuthStore) InsertAuthSession(_ context.Context, token, did, username, name string, ttl time.Duration) error {
	f.sessionInsertCalls = append(f.sessionInsertCalls, insertSessionCall{token, did, username, name, ttl})
	return f.sessionInsertErr
}
func (f *fakeAuthStore) GetAuthSession(_ context.Context, _ string) (*db.AuthSession, error) {
	return f.sessionGetRet, f.sessionGetErr
}
func (f *fakeAuthStore) ConsumeAuthSession(_ context.Context, _ string) (*db.AuthSession, error) {
	return f.sessionConsumeRet, f.sessionConsumeErr
}

// fakeJWT implements api.JWTVerifier.
type fakeJWT struct {
	claims *auth.Claims
	err    error
}

func (f *fakeJWT) Verify(_ string) (*auth.Claims, error) { return f.claims, f.err }

// fakeIDGen returns a single fixed ID.
type fakeIDGen struct{ ID string }

func (f fakeIDGen) NewUUID() string { return f.ID }

var errSaveNotFound = db.ErrSaveNotFound

var _ = errors.Is
