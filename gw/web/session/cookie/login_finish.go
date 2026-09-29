package cookie

import "net/http"

// FinishLogin completes a browser login for an already-stored user session:
// sets the session cookie carrying sealedCookieValue (NewUserSessionID), then
// redirects — to the intended URI saved by the login redirect, if one is
// present, else to successPath (303). Nothing in it can fail: every step of a
// login that can comes before StoreUserSession.
func FinishLogin(w http.ResponseWriter, r *http.Request, mgr *SessionManager, sealedCookieValue, successPath string) {
	mgr.SetUserSessionCookie(w, sealedCookieValue)
	if TryRedirectIfIntendedURICookie(w, r, mgr.Conf.UserSession.LoginPath) {
		return
	}
	http.Redirect(w, r, successPath, http.StatusSeeOther)
}
