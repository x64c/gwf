package errs

import (
	"fmt"
	"maps"
)

// ErrorSet is an O(1) membership set of error sentinels. Build it from
// framework-provided *Error symbols so app code never pins a raw int code
// (codes are internal and renumber freely).
//
// Membership (Has) answers the same identity question Is does, pluralized —
// Name AND Code — so the two can never disagree about whether an error "is"
// one of a set. The map is keyed by code, which stays unambiguous because Add
// refuses two members sharing one; Has then verifies the Name. Code-only
// membership, where wanted, is HasCode — named for what it does, like
// IsSameCode.
type ErrorSet struct {
	codes map[int]*Error
}

func NewErrorSet(es ...*Error) (*ErrorSet, error) {
	s := &ErrorSet{codes: make(map[int]*Error, len(es))}
	for _, e := range es {
		if err := s.Add(e); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Add inserts e, returning an error if its code is already in the set. The
// refusal is deliberate beyond keeping the map unambiguous: Code is what the
// wire serializes, so two errors sharing one is a system-level ambiguity, and
// sets are built at boot — this surfaces the collision loudly at startup.
func (s *ErrorSet) Add(e *Error) error {
	if e == nil {
		return nil
	}
	if existing, dup := s.codes[e.Code]; dup {
		return fmt.Errorf("duplicate code %d - %q, %q", e.Code, existing.Name, e.Name)
	}
	s.codes[e.Code] = e
	return nil
}

// Del removes e's code from the set; nil or absent is a no-op.
func (s *ErrorSet) Del(e *Error) {
	if e == nil {
		return
	}
	delete(s.codes, e.Code)
}

// Clone returns an independent copy — mutate the clone to derive a set.
func (s *ErrorSet) Clone() *ErrorSet {
	return &ErrorSet{codes: maps.Clone(s.codes)}
}

// Has reports whether e is a member — identity is Name AND Code, matching Is.
// nil → false.
func (s *ErrorSet) Has(e *Error) bool {
	if e == nil {
		return false
	}
	member, ok := s.codes[e.Code]
	return ok && member.Name == e.Name
}

// HasCode reports whether a member carries this raw code — the declared
// code-only membership check (the HasCode:Has relation is IsSameCode:Is).
func (s *ErrorSet) HasCode(code int) bool {
	_, ok := s.codes[code]
	return ok
}
