package handlerwrappers

import (
	"log"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session/bearer"
)

// BearerUserSession validates "Authorization: Bearer ..." against KVDB,
// requires the session to be user-bound, optionally restricts to specific
// session groups, attaches *bearer.UserSessionData[UID] to ctx, and forwards.
//
// Returns 401 if the token is missing or invalid. Returns 403 if the session is
// userless (wrong shape) or its group isn't in AllowedGroups.
type BearerUserSession[UID comparable] struct {
	AppProvider   framework.AppProviderFunc
	ParseUID      func(string) (UID, error)
	AllowedGroups []string // empty = accept any group from config
}

func (m *BearerUserSession[UID]) Wrap(inner http.Handler) http.Handler {
	appCore := m.AppProvider().AppCore()
	sessSvc := appCore.SessionService
	if sessSvc == nil || sessSvc.BearerSessionManager == nil {
		log.Fatal("[ERROR] BearerUserSession - session manager missing")
	}
	mgr := sessSvc.BearerSessionManager

	// Boot-time: build allowed-groups lookup map once, captured in closure.
	var allowedGroupsSet map[string]struct{}
	if len(m.AllowedGroups) == 0 {
		allowedGroupsSet = make(map[string]struct{}, len(mgr.GroupConfs))
		for groupName := range mgr.GroupConfs {
			allowedGroupsSet[groupName] = struct{}{}
		}
	} else {
		allowedGroupsSet = make(map[string]struct{}, len(m.AllowedGroups))
		for _, g := range m.AllowedGroups {
			allowedGroupsSet[g] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sessSvc.Serving(mgr) {
			responses.WriteErrorJSON(w, http.StatusServiceUnavailable, errs.SessionServiceUnavailable.WithDetail("bearer user session"))
			return
		}
		ctx := r.Context()
		accessToken := security.ExtractBearerToken(r.Header.Get("Authorization"))
		if accessToken == "" {
			responses.WriteErrorJSON(w, http.StatusUnauthorized, errs.AccessTokenNotFound.WithDetail("missing Authorization Bearer header"))
			return
		}

		// Two-hop: access token hash → sid → umbrella row
		hash := security.HashHexSHA256(accessToken)
		sid, found, err := mgr.KVDB.Get(ctx, mgr.AccessTokenRowKey(hash))
		if err != nil {
			responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.Wrap(err))
			return
		}
		if !found {
			responses.WriteErrorJSON(w, http.StatusUnauthorized, errs.AccessTokenNotFound)
			return
		}
		row, err := mgr.FetchSession(ctx, sid)
		if err != nil {
			responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("fetching session row").WithCause(err))
			return
		}
		if row == nil {
			responses.WriteErrorJSON(w, http.StatusUnauthorized, errs.AccessTokenNotFound)
			return
		}

		// Shape check: this middleware requires user-bound
		if row.UID == "" {
			responses.WriteErrorJSON(w, http.StatusForbidden, errs.BearerSessionShapeMismatch.WithDetail("user-bound required"))
			return
		}

		// Group filter
		if _, ok := allowedGroupsSet[row.GroupName]; !ok {
			responses.WriteErrorJSON(w, http.StatusForbidden, errs.BearerSessionGroupNotAllowed.WithDetail(row.GroupName))
			return
		}

		uid, err := m.ParseUID(row.UID)
		if err != nil {
			responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("parse uid").WithCause(err))
			return
		}

		sd := &bearer.UserSessionData[UID]{
			ID:        sid,
			UIDStr:    row.UID,
			UID:       uid,
			ClientID:  row.ClientID,
			GroupName: row.GroupName,
			Mgr:       mgr,
		}
		inner.ServeHTTP(w, r.WithContext(bearer.WithUserSessionData(ctx, sd)))
	})
}
