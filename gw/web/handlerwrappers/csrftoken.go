package handlerwrappers

import (
	"crypto/subtle"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session/cookie"
)

// CSRFToken verifies that the request carries a CSRF token matching the one
// stored on the request's cookie session. Wire on routes/groups that require
// CSRF protection (typically state-changing endpoints under cookie auth).
//
// Submitted token is read from the header (cookie.CSRFHeaderName); if blank,
// falls back to the form field (cookie.CSRFFormFieldName).
//
// CSRF defends against ambient-credential attacks; bearer sessions don't need it.
type CSRFToken[UID comparable] struct{}

func (m *CSRFToken[UID]) Wrap(inner http.Handler) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqTkn := r.Header.Get(cookie.CSRFHeaderName)
		if reqTkn == "" {
			reqTkn = r.PostFormValue(cookie.CSRFFormFieldName)
		}
		if reqTkn == "" {
			responses.WriteErrorJSON(w, http.StatusForbidden, errs.CSRFTokenNotFound)
			return
		}
		sd, ok := cookie.UserSessionDataFromContext[UID](r.Context())
		if !ok {
			responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.DataMissingInContext.WithDetail("SessionData"))
			return
		}
		if subtle.ConstantTimeCompare([]byte(reqTkn), []byte(sd.CSRFTkn)) != 1 {
			responses.WriteErrorJSON(w, http.StatusForbidden, errs.InvalidCSRFToken)
			return
		}
		inner.ServeHTTP(w, r)
	}), nil
}
