package cookie

import (
	"net/http"
	"net/url"
	"strings"
)

// SetIntendedURICookie saves r's URI for the login to return to. The login
// returns with a GET, so only a GET or HEAD request's URI is saved: a refused
// POST (e.g. a form sent after the session expired) saves nothing, and the
// login goes to its success path.
func SetIntendedURICookie(w http.ResponseWriter, r *http.Request, maxAge int) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     UserIntendedURICookieName,
		Value:    url.QueryEscape(r.URL.RequestURI()),
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func RemoveIntendedURICookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     UserIntendedURICookieName,
		Path:     "/",
		MaxAge:   -1, // Delete
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// TryRedirectIfIntendedURICookie tries to redirect if IntendedURICookie is set, returning true if redirected.
// loginPath: e.g. core.UserCookieSessionManager.Conf.LoginPath
func TryRedirectIfIntendedURICookie(w http.ResponseWriter, r *http.Request, loginPath string) bool {
	intendedUriCookie, err := r.Cookie(UserIntendedURICookieName)
	if err != nil {
		return false // no cookie [http.ErrNoCookie]
	}

	RemoveIntendedURICookie(w)

	decodedURI, err := url.QueryUnescape(intendedUriCookie.Value)
	if err != nil || decodedURI == "" || decodedURI == loginPath {
		return false // malformed or meaningless value
	}

	if !isLocalPath(decodedURI) {
		return false // only a path on this site, never another host
	}

	http.Redirect(w, r, decodedURI, http.StatusSeeOther)
	return true
}

// isLocalPath reports whether v is a path on this site: it starts with exactly
// one "/" — not "//" or "/\", which a browser reads as another host — and parses
// with no scheme and no host. url.Parse also refuses control characters, which
// a browser would strip (turning "/\t/host" into "//host").
func isLocalPath(v string) bool {
	if !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") || strings.HasPrefix(v, `/\`) {
		return false
	}
	u, err := url.Parse(v)
	return err == nil && u.Scheme == "" && u.Host == ""
}
