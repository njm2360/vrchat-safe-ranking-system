package api

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

type stubRankingFetcher struct {
	rows  []db.RankingRow
	err   error
	calls atomic.Int32
	block chan struct{} // 非 nil なら Ranking は close されるまでブロック
}

func (s *stubRankingFetcher) Ranking(_ context.Context, _ int, _ bool) ([]db.RankingRow, error) {
	s.calls.Add(1)
	if s.block != nil {
		<-s.block
	}
	return s.rows, s.err
}

func (s *stubRankingFetcher) Calls() int { return int(s.calls.Load()) }

func makeRows(n int) []db.RankingRow {
	rows := make([]db.RankingRow, n)
	for i := range rows {
		rows[i] = db.RankingRow{Rank: i + 1, Score: int64(n - i)}
	}
	return rows
}

func newCache(src rankingFetcher, ttl time.Duration) *rankingCache {
	return &rankingCache{src: src, ttl: ttl, fetchN: 1000}
}

func TestRankingCache_HitDoesNotRefetch(t *testing.T) {
	f := &stubRankingFetcher{rows: makeRows(5)}
	rc := newCache(f, time.Minute)

	for i := 0; i < 3; i++ {
		if _, err := rc.get(context.Background(), 3, false); err != nil {
			t.Fatal(err)
		}
	}
	if f.Calls() != 1 {
		t.Errorf("calls = %d, want 1", f.Calls())
	}
}

func TestRankingCache_TTLRefetches(t *testing.T) {
	f := &stubRankingFetcher{rows: makeRows(5)}
	rc := newCache(f, 20*time.Millisecond)

	if _, err := rc.get(context.Background(), 3, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := rc.get(context.Background(), 3, false); err != nil {
		t.Fatal(err)
	}
	if f.Calls() != 2 {
		t.Errorf("calls = %d, want 2", f.Calls())
	}
}

func TestRankingCache_SliceLimitAndPreservesRanks(t *testing.T) {
	f := &stubRankingFetcher{rows: makeRows(50)}
	rc := newCache(f, time.Minute)

	got, err := rc.get(context.Background(), 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	if got[0].Rank != 1 || got[4].Rank != 5 {
		t.Errorf("ranks not preserved: %+v", got)
	}
}

func TestRankingCache_LimitLargerThanRowsReturnsAll(t *testing.T) {
	f := &stubRankingFetcher{rows: makeRows(3)}
	rc := newCache(f, time.Minute)

	got, err := rc.get(context.Background(), 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
}

func TestRankingCache_SeparateSlotsByVerifiedOnly(t *testing.T) {
	f := &stubRankingFetcher{rows: makeRows(5)}
	rc := newCache(f, time.Minute)

	if _, err := rc.get(context.Background(), 3, false); err != nil {
		t.Fatal(err)
	}
	if _, err := rc.get(context.Background(), 3, true); err != nil {
		t.Fatal(err)
	}
	if f.Calls() != 2 {
		t.Errorf("calls = %d, want 2 (separate slots per verifiedOnly)", f.Calls())
	}

	// それぞれのスロットは独立にキャッシュされている
	if _, err := rc.get(context.Background(), 3, false); err != nil {
		t.Fatal(err)
	}
	if _, err := rc.get(context.Background(), 3, true); err != nil {
		t.Fatal(err)
	}
	if f.Calls() != 2 {
		t.Errorf("calls = %d, want 2 (both slots cached)", f.Calls())
	}
}

func TestRankingCache_SingleFlightCoalesces(t *testing.T) {
	f := &stubRankingFetcher{rows: makeRows(5), block: make(chan struct{})}
	rc := newCache(f, time.Minute)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = rc.get(context.Background(), 3, false)
		}()
	}
	// 全 goroutine が singleflight に合流できるよう少し待ってからアンブロック。
	time.Sleep(30 * time.Millisecond)
	close(f.block)
	wg.Wait()

	if f.Calls() != 1 {
		t.Errorf("calls = %d, want 1 (singleflight should coalesce)", f.Calls())
	}
}

func TestRankingCache_ErrorNotCached(t *testing.T) {
	f := &stubRankingFetcher{err: errors.New("boom")}
	rc := newCache(f, time.Minute)

	if _, err := rc.get(context.Background(), 3, false); err == nil {
		t.Fatal("expected error")
	}
	if _, err := rc.get(context.Background(), 3, false); err == nil {
		t.Fatal("expected error on retry")
	}
	if f.Calls() != 2 {
		t.Errorf("calls = %d, want 2 (error must not be cached)", f.Calls())
	}
}

func TestRankingCache_NilRowsReturnsNil(t *testing.T) {
	f := &stubRankingFetcher{rows: nil}
	rc := newCache(f, time.Minute)

	got, err := rc.get(context.Background(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
}
