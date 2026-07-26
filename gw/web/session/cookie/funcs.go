package cookie

import (
	"net/http"
	"net/url"
)

func SetIntendedURICookie(w http.ResponseWriter, r *http.Request, maxAge int) {
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

	if parsedURL, err := url.Parse(decodedURI); err != nil || parsedURL.IsAbs() {
		return false // reject external redirect
	}

	http.Redirect(w, r, decodedURI, http.StatusSeeOther)
	return true
}
