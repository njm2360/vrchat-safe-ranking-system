// Package clock provides a small abstraction over time.Now so that callers
// can inject a fake clock in tests.
package clock

import "time"

type Clock interface {
	Now() time.Time
}

// System is the production Clock backed by time.Now.
type System struct{}

func (System) Now() time.Time { return time.Now() }

// Fake is a manually-advanced Clock for tests.
type Fake struct {
	NowVal time.Time
}

// NewFake returns a Fake initialised to t. If t is zero, uses 2025-01-01 UTC.
func NewFake(t time.Time) *Fake {
	if t.IsZero() {
		t = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return &Fake{NowVal: t}
}

func (f *Fake) Now() time.Time { return f.NowVal }

func (f *Fake) Advance(d time.Duration) { f.NowVal = f.NowVal.Add(d) }
