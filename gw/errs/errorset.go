package errs

import (
	"fmt"
	"maps"
)

// ErrorSet is an O(1) membership set of error sentinels, keyed internally by
// logic code. Build it from framework-provided *Error symbols so app code never
// pins a raw int code (codes are internal and renumber freely).
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

// Add inserts e, returning an error if its code is already in the set.
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

// Has reports whether e's code is in the set. nil → false.
func (s *ErrorSet) Has(e *Error) bool {
	if e == nil {
		return false
	}
	_, ok := s.codes[e.Code]
	return ok
}

// HasCode is the same check when you only have a raw code.
func (s *ErrorSet) HasCode(code int) bool {
	_, ok := s.codes[code]
	return ok
}
