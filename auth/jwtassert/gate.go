package jwtassert

import (
	"errors"
	"net/http"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/responses"
)

// Gate is the handler wrapper for this method: it authenticates the request
// by its assertion through Verifier and, on admission, places the resulting
// authn.VerifiedIdentity in the request context for the wrapped handler.
//
// Refusals are written as JSON and the wrapped handler is not called: 401
// with the Verifier's sentinel (AssertionNotFound, InvalidAssertion,
// AssertionReplayed, AssertionClientUnknown), 413 RequestBodyTooLarge when
// the body exceeds the client's MaxBodyBytes.
type Gate struct {
	Verifier *Verifier
}

func (m *Gate) Wrap(inner http.Handler) (http.Handler, error) {
	if m.Verifier == nil {
		return nil, errors.New("jwtassert.Gate: Verifier required")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _, e := m.Verifier.VerifyRequest(r)
		if e != nil {
			status := http.StatusUnauthorized
			if e.IsSameCode(errs.RequestBodyTooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			responses.WriteErrorJSON(w, status, e)
			return
		}
		inner.ServeHTTP(w, r.WithContext(authn.WithVerifiedIdentity(r.Context(), id)))
	}), nil
}
