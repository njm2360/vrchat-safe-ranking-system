package idgen

import (
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"
)

type Generator interface {
	NewUUID() string
}

type Real struct{}

func (Real) NewUUID() string { return uuid.NewString() }

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
