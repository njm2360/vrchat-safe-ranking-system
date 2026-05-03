// Package idgen provides UUID generation as an injectable dependency so tests
// can use deterministic IDs.
package idgen

import (
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"
)

type Generator interface {
	NewUUID() string
}

// Real produces RFC4122 v4 UUIDs.
type Real struct{}

func (Real) NewUUID() string { return uuid.NewString() }

// Sequential returns deterministic IDs for tests, e.g. "ticket-0001",
// "ticket-0002", ...
type Sequential struct {
	Prefix string
	n      atomic.Uint64
}

func NewSequential(prefix string) *Sequential {
	return &Sequential{Prefix: prefix}
}

func (s *Sequential) NewUUID() string {
	n := s.n.Add(1)
	return fmt.Sprintf("%s-%04d", s.Prefix, n)
}
