package errs

import "errors"

type Error struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Code    int    `json:"code"` // logic error code
	Cause   error  `json:"-"`    // original error, not serialized
}

func (e *Error) Error() string {
	return e.Message
}

// Is implements stdlib errors.Is interface. Walks the target chain via AsType
// to find an *Error and compare identity — Name AND Code. Handles bare
// sentinels and wrapped sentinels uniformly (the wrap family copies both
// fields).
//
// Identity is the pair because neither field alone is owned: the code space
// has no allocation authority (an app sentinel can collide with a framework
// code — and every legacy zero-code error would equal every other), and names
// can repeat across packages just as freely. Requiring both means only true
// copies of a sentinel compare equal, and a half-renumbered pair fails closed
// (matches nothing) instead of matching someone else's error. Code-only
// comparison, where wanted, is IsSameCode — named for what it does.
func (e *Error) Is(target error) bool {
	if t, ok := errors.AsType[*Error](target); ok {
		return e.Code == t.Code && e.Name == t.Name
	}
	return false
}

// IsSameCode reports whether this error and target share the same logic Code.
// Fast path for when you already have *Error directly — no interface boxing,
// no chain walking, just an int comparison. Prefer this in framework/app code
// where *errs.Error is passed around typed.
func (e *Error) IsSameCode(target *Error) bool {
	return e.Code == target.Code
}

// WithDetail returns a new Error with the detail appended to the message.
func (e *Error) WithDetail(detail string) *Error {
	return &Error{Name: e.Name, Code: e.Code, Message: e.Message + ": " + detail, Cause: e.Cause}
}

// WithCause returns a new Error preserving the existing Message but attaching
// the given error as Cause. Useful when the message is built separately
// (e.g. via WithDetail) but we still want errors.Is/As to walk to the original.
func (e *Error) WithCause(err error) *Error {
	return &Error{Name: e.Name, Code: e.Code, Message: e.Message, Cause: err}
}

// Wrap returns a new Error carrying the same Code, with the underlying error
// preserved as Cause and its message appended to the base message.
// Used at boundaries where a stdlib/external error enters the errs system.
func (e *Error) Wrap(err error) *Error {
	return &Error{Name: e.Name, Code: e.Code, Message: e.Message + ": " + err.Error(), Cause: err}
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// AsStructured bridges a plain error to *Error.
// If err already carries an *Error in its chain, returns that *Error as-is.
// Otherwise returns fallback.Wrap(err) so the caller always has a structured error.
// Useful at handler boundaries while framework funcs still return the plain error interface.
func AsStructured(err error, fallback *Error) *Error {
	if e, ok := errors.AsType[*Error](err); ok {
		return e
	}
	return fallback.Wrap(err)
}
