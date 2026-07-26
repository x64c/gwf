package cookie

import (
	"context"
	"fmt"
	"html/template"
)

const (
	CSRFHeaderName    = "X-CSRF-Token"
	CSRFFormFieldName = "_csrf_token"
)

// UserCSRFTokenFromContext returns the CSRF token attached to the request's
// user cookie session, or ("", false) if no UserSessionData is present.
func UserCSRFTokenFromContext[UID comparable](ctx context.Context) (string, bool) {
	sd, ok := UserSessionDataFromContext[UID](ctx)
	if !ok {
		return "", false
	}
	return sd.CSRFTkn, true
}

// AnonymousCSRFTokenFromContext returns the CSRF token attached to the request's
// anonymous cookie session, or ("", false) if no AnonymousSessionData is present.
func AnonymousCSRFTokenFromContext(ctx context.Context) (string, bool) {
	sd, ok := AnonymousSessionDataFromContext(ctx)
	if !ok {
		return "", false
	}
	return sd.CSRFTkn, true
}

// CSRFHiddenInputHTML returns the hidden <input> element carrying the CSRF
// token under CSRFFormFieldName, suitable for embedding in <form> templates.
//
// Returns template.HTML so html/template renders it raw (without escaping the
// angle brackets).
func CSRFHiddenInputHTML(token string) template.HTML {
	return template.HTML(fmt.Sprintf(`<input type="hidden" name=%q value=%q>`, CSRFFormFieldName, token))
}
