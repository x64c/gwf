# clock

A clock that advances in discrete ticks.

Go measures time in nanoseconds. Most systems we talk to do not — one stores
whole milliseconds, another whole seconds. Such a system is a world with its
own tick, and in that world there is no time between marks.

This package exists so that crossing into such a world is something you *do*,
visibly, rather than something that happens to your values on the way past.

## The model

**A clock reads.** Every judgment about time is made at some precision and
answered by where the hand is, not by how much time has really passed. Am I
overdue? Not if the hand has not reached the mark. What time is it? 3:15 —
meaning the hand is at 3:15 and has not landed on 3:16. We do this constantly
without calling it approximation, because it is what reading a clock means.

Three rules follow, and together they are the whole package:

1. **An instant renders to the last mark the clock has ticked** — `Read`.
   Truncation, and not as a choice: a clock displaying seconds shows 12 for the
   whole of [12, 13) because the 13th tick has not happened.

2. **Durations are not rendered** — `ReadBetween`. A duration in that world is
   the difference of two already-read instants, so it is a whole number of ticks
   by construction. There is nothing left to round, which is why this package
   has no `Round` or `Ceil`: the question they would answer does not arise once
   the conversion sits at the boundary.

3. **A mark is not passed until the clock leaves it.** Whatever holds until mark
   T holds for the whole of T and gives way at T+1. This one is a *requirement
   on the systems we describe*, not a fact this package can enforce — but every
   guarantee built on a rendered deadline depends on it.

## Why reading both ends is not the same as rendering the difference

```
t1 = 12.6s, t2 = 13.4s, real Δ = 0.8s

endpoints into a 1s world → 12, 13 → Δ = 1 tick
the difference into that world →  0.8 → 0 ticks
```

They disagree, and they disagree under rounding too. **The endpoints are the
truth**: in that world t1 and t2 are what its clock reads, and the difference
follows. The 0.8s never existed there.

So the same real interval reads differently depending where it falls — on a 1s
clock, 0.8s reads as 0 inside one second and as 1 across a mark. That is not
error, it is the point. `ReadBetween` answers *how many times did this clock
tick between these two instants*, which is the right question for "have we
crossed a mark", "is this a different second", "has the deadline passed" — and
the wrong one for "how long did this take".

## API

```go
c := clock.New(time.Millisecond)   // panics on a non-positive tick

c.Precision()                      // the tick
c.Read(t)                          // the mark this clock shows at instant t
c.ReadBetween(from, to)            // ticks between two instants, as this clock sees them
c.ReadDurationFrom(from, d)        // ticks a stretch of d beginning at from occupies
```

`ReadDurationFrom` is anchored on purpose: `d` alone cannot answer, because
where a stretch starts decides how many marks it crosses.

`Clock` is a value — one `time.Duration`, so copying it costs exactly what
copying a pointer to it would, without the indirection, and two copies can
never diverge. `New` is the only way to get a usable one; the zero `Clock` has
no precision and is documented as unusable.

Results are whole ticks within `time.Duration`'s range: instants more than
roughly ±292 years apart overflow it, capping the value as `Time.Sub` does.

## Where it is used

`kvdbs.DB` publishes one through `Clock()`, so a caller can place a deadline
with the same rules the implementation uses instead of re-deriving the
arithmetic. Any component with its own tick — a scheduler, a lease, a rate
window — is a candidate for the same treatment.
