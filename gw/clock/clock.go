// Package clock models a clock that advances in discrete ticks.
//
// Every judgment about time is made at some precision and answered by where the
// hand is, not by how much time has really passed. Am I overdue? Not if the
// hand has not reached the mark. What time is it? 3:15 — meaning the hand is at
// 3:15 and has not landed on 3:16. We do this constantly without calling it
// approximation, because it is simply what reading a clock means.
//
// Go measures in nanoseconds, which hides that. Most systems we talk to do not:
// one stores whole milliseconds, another whole seconds. Such a system is a
// world with its own tick, and there is no time between its marks.
package clock

import "time"

// Clock is a clock whose smallest representable difference is its precision.
// The zero Clock is not usable; build one with New.
type Clock struct {
	precision time.Duration
}

// New returns a Clock ticking at precision, which must be positive.
//
// It panics otherwise, because a non-positive tick is a programming error in
// the caller's own source — the value is a constant of whatever system the
// Clock describes, so a wrong one is fixed by editing code, not by handling an
// error at runtime.
func New(precision time.Duration) Clock {
	if precision <= 0 {
		panic("clock: precision must be positive, got " + precision.String())
	}
	return Clock{precision: precision}
}

// Precision is the clock's tick: the smallest difference it can represent.
func (c Clock) Precision() time.Duration {
	return c.precision
}

// Read is what this clock shows at instant t: the last mark it has ticked.
// A clock reading seconds shows 12 for the whole of [12, 13) — the 13th tick
// has not happened yet — so this truncates rather than rounding.
func (c Clock) Read(t time.Time) time.Time {
	return t.Truncate(c.precision)
}

// ReadBetween is how many ticks this clock advances between two instants — not
// the real interval rounded. The same interval reads differently depending
// where it falls: on a 1s clock, 0.8s reads as 0 inside one second and as 1
// across a mark. That is not error but the point — "am I late?" turns on
// whether the hand reached the mark, not on elapsed time. Negative when to
// precedes from.
//
// The result is always a whole number of ticks, within time.Duration's range:
// instants more than roughly ±292 years apart overflow it, capping the value
// as Time.Sub does.
func (c Clock) ReadBetween(from, to time.Time) time.Duration {
	return c.Read(to).Sub(c.Read(from))
}

// ReadDurationFrom is how many ticks a stretch of d beginning at from occupies
// on this clock — ReadBetween of its two ends.
//
// It is anchored on purpose, and that is the whole of it: d alone cannot answer,
// because where a stretch starts decides how many marks it crosses. On a 1s
// clock, 0.8s from 12.6 spans a tick and the same 0.8s from 12.1 spans none.
// A stretch shorter than a tick spans one or none depending on where it falls;
// a d of zero always spans none.
func (c Clock) ReadDurationFrom(from time.Time, d time.Duration) time.Duration {
	return c.ReadBetween(from, from.Add(d))
}
