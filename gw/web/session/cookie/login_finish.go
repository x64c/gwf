package cookie

import "net/http"

// FinishLogin completes a browser login for an already-stored user session:
// sets the session cookie for sessionID, then redirects — to the intended
// URI saved by the login redirect, if one is present, else to successPath
// (303). Returns the cookie-setting error unwritten; the caller answers it.
//
// StoreUserSession is deliberately a separate step: a flow that also stores
// upstream tokens on the row does so between the two.
func FinishLogin(w http.ResponseWriter, r *http.Request, mgr *SessionManager, sessionID, successPath string) error {
	if err := mgr.SetUserSessionCookie(w, sessionID); err != nil {
		return err
	}
	if TryRedirectIfIntendedURICookie(w, r, mgr.Conf.UserSession.LoginPath) {
		return nil
	}
	http.Redirect(w, r, successPath, http.StatusSeeOther)
	return nil
}
