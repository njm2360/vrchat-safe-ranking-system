package api

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

const (
	rankingCacheTTL  = 30 * time.Second
	rankingCacheSize = 1000
)

type rankingFetcher interface {
	Ranking(ctx context.Context, limit int, verifiedOnly bool) ([]db.RankingRow, error)
}

type rankingCacheEntry struct {
	rows      []db.RankingRow
	expiresAt time.Time
}

type rankingCache struct {
	src    rankingFetcher
	ttl    time.Duration
	fetchN int

	sf      singleflight.Group
	mu      sync.RWMutex
	entries [2]rankingCacheEntry // 0=!verifiedOnly, 1=verifiedOnly
}

func newRankingCache(src rankingFetcher) *rankingCache {
	return &rankingCache{src: src, ttl: rankingCacheTTL, fetchN: rankingCacheSize}
}

func (rc *rankingCache) get(ctx context.Context, limit int, verifiedOnly bool) ([]db.RankingRow, error) {
	idx, key := 0, "u"
	if verifiedOnly {
		idx, key = 1, "v"
	}

	rc.mu.RLock()
	e := rc.entries[idx]
	rc.mu.RUnlock()
	if time.Now().Before(e.expiresAt) {
		return sliceLimit(e.rows, limit), nil
	}

	v, err, _ := rc.sf.Do(key, func() (any, error) {
		rc.mu.RLock()
		e := rc.entries[idx]
		rc.mu.RUnlock()
		if time.Now().Before(e.expiresAt) {
			return e.rows, nil
		}
		rows, err := rc.src.Ranking(ctx, rc.fetchN, verifiedOnly)
		if err != nil {
			return nil, err
		}
		rc.mu.Lock()
		rc.entries[idx] = rankingCacheEntry{rows: rows, expiresAt: time.Now().Add(rc.ttl)}
		rc.mu.Unlock()
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	return sliceLimit(v.([]db.RankingRow), limit), nil
}

func sliceLimit(rows []db.RankingRow, limit int) []db.RankingRow {
	if limit >= len(rows) {
		return rows
	}
	return rows[:limit]
}
