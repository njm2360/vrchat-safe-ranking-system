package clock

import "time"

type Clock interface {
	Now() time.Time
}

type System struct{}

func (System) Now() time.Time { return time.Now() }

type Fake struct {
	NowVal time.Time
}

func NewFake(t time.Time) *Fake {
	if t.IsZero() {
		t = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return &Fake{NowVal: t}
}

func (f *Fake) Now() time.Time { return f.NowVal }

func (f *Fake) Advance(d time.Duration) { f.NowVal = f.NowVal.Add(d) }
