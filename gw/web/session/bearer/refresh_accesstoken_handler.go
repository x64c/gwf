package bearer

import (
	"encoding/json/v2"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/responses"
)

// RefreshAccessTokenHandler is the HTTP handler for the framework's
// access-token refresh endpoint. Reads a JSON body containing a refresh token,
// calls SessionManager.ExtendSession to rotate the pair, and writes the new
// pair as JSON.
//
// App side registers it like:
//
//	router.Handle("POST <path>", &bearer.RefreshAccessTokenHandler{
//	    SessionManager: app.SessionService.BearerSessionManager,
//	})
type RefreshAccessTokenHandler struct {
	SessionManager *SessionManager
}

func (h *RefreshAccessTokenHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	ctx := r.Context()

	var reqBody security.RefreshAccessTokenRequestBody
	if err := json.UnmarshalRead(r.Body, &reqBody); err != nil {
		responses.WriteErrorJSON(w, http.StatusBadRequest, errs.JSONUnmarshalFailed.WithCause(err))
		return
	}

	newAccessToken, newRefreshToken, extErr := h.SessionManager.ExtendSession(ctx, reqBody.RefreshToken)
	if extErr != nil {
		responses.WriteErrorJSON(w, http.StatusUnauthorized, extErr)
		return
	}

	responses.EncodeWriteJSON(w, http.StatusOK, map[string]string{
		"access_token":  newAccessToken,
		"refresh_token": newRefreshToken,
	})
}
