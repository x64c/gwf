package passwd

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"mime"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
)

// The members a login request carries its name and password in — form fields of a form post,
// or members of a JSON object: {"name": …, "password": …}.
const (
	NameField     = "name"
	PasswordField = "password"
)

// readLoginRequest reads the name and password a login request carries: a form post
// (application/x-www-form-urlencoded — a browser form's own encoding) or a JSON body
// (application/json). A JSON body is put back after reading, so a throttle key and the handler can
// both read it; a form is parsed once and kept by net/http.
//
//   - Returns: the name and the password — the caller clears the password; or 400
//     errs.JSONUnmarshalFailed for a JSON body that is not the object above, or errs.InvalidLoginRequest
//     for another content type or an unreadable form.
//   - Uses: mime.ParseMediaType, http.Request.PostFormValue, json.Unmarshal.
func readLoginRequest(r *http.Request) (string, []byte, *errs.Error) {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch mediaType {
	case "application/x-www-form-urlencoded":
		if err := r.ParseForm(); err != nil {
			return "", nil, errs.InvalidLoginRequest.WithDetail("unreadable form").WithCause(err)
		}
		return r.PostFormValue(NameField), []byte(r.PostFormValue(PasswordField)), nil
	case "application/json":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return "", nil, errs.InvalidLoginRequest.WithDetail("unreadable body").WithCause(err)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var req struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return "", nil, errs.JSONUnmarshalFailed.WithCause(err)
		}
		return req.Name, []byte(req.Password), nil
	}
	return "", nil, errs.InvalidLoginRequest.WithDetail("a login is a form post or a JSON body")
}

// loginStatus is the HTTP status a login refusal is answered with.
//
//   - Returns: 401 for errs.InvalidCredentials, 503 for errs.PasswordCheckBusy, 400 for a request
//     that could not be read, 500 otherwise.
func loginStatus(e *errs.Error) int {
	switch {
	case e.IsSameCode(errs.InvalidCredentials):
		return http.StatusUnauthorized
	case e.IsSameCode(errs.PasswordCheckBusy):
		return http.StatusServiceUnavailable
	case e.IsSameCode(errs.InvalidLoginRequest), e.IsSameCode(errs.JSONUnmarshalFailed):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
