package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func TestRanking_ReturnsJSON(t *testing.T) {
	saves := &fakeSaveStore{
		rankingRet: []db.RankingRow{
			{Rank: 1, DisplayName: "alice", Score: 999},
			{Rank: 2, DisplayName: "bob", Score: 100},
		},
	}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, body := get(t, h, "/ranking?limit=10")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var rows []db.RankingRow
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, body)
	}
	if len(rows) != 2 || rows[0].DisplayName != "alice" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestRanking_EmptyEncodesAsEmptyArray(t *testing.T) {
	saves := &fakeSaveStore{rankingRet: nil}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	_, body := get(t, h, "/ranking")
	if body != "[]\n" && body != "[]" {
		t.Errorf("body = %q, want empty array", body)
	}
}
