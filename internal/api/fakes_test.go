package api_test

import (
	"context"
	"errors"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

// fakeTicketStore implements api.TicketStore.
type fakeTicketStore struct {
	allowed     bool
	checkErr    error
	insertCalls []insertTicketCall
	insertErr   error
	upsertCalls int
	upsertErr   error
}

type insertTicketCall struct {
	UUID        string
	DisplayName string
	TTL         time.Duration
}

func (f *fakeTicketStore) InsertTicket(_ context.Context, uuid, dn string, ttl time.Duration) error {
	f.insertCalls = append(f.insertCalls, insertTicketCall{uuid, dn, ttl})
	return f.insertErr
}
func (f *fakeTicketStore) CheckChallengeRate(_ context.Context, _ string, _ time.Duration) (time.Time, bool, error) {
	return time.Time{}, f.allowed, f.checkErr
}
func (f *fakeTicketStore) UpsertChallengeRate(_ context.Context, _ string) error {
	f.upsertCalls++
	return f.upsertErr
}

// fakeSaveStore implements api.SaveStore.
type fakeSaveStore struct {
	saveCalls    []saveCall
	saveErr      error
	saveID       int64
	latestRet    *db.SaveEntry
	latestErr    error
	rankingRet   []db.RankingRow
	rankingErr   error
}

type saveCall struct {
	DisplayName string
	Score       int64
	JTI         string
}

func (f *fakeSaveStore) Save(_ context.Context, dn string, score int64, jti string) (int64, error) {
	f.saveCalls = append(f.saveCalls, saveCall{dn, score, jti})
	return f.saveID, f.saveErr
}
func (f *fakeSaveStore) GetLatestSave(_ context.Context, _ string) (*db.SaveEntry, error) {
	return f.latestRet, f.latestErr
}
func (f *fakeSaveStore) Ranking(_ context.Context, _ int) ([]db.RankingRow, error) {
	return f.rankingRet, f.rankingErr
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

// errSaveNotFound is a thin alias so tests can use the sentinel without importing db.
var errSaveNotFound = db.ErrSaveNotFound

var _ = errors.Is
