package oidc

import (
	"net/http"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/responses"
)

// AuthCodeRequestHandler is the HTTP handler for a flow's login endpoint: it
// issues the flow ticket and redirects the user-agent to the provider's
// authorization URL. Serves both Delegated and Direct Code Exchange — the
// initiate half is the same; only the verify half differs.
//
// App side registers it like:
//
//	router.Handle("GET <login path>", &oidc.AuthCodeRequestHandler{
//	    Provider:    &app.LoginConf.Provider,
//	    RedirectURI: app.LoginConf.RedirectURI,
//	    Flow:        app.AuthnFlow,
//	})
//
// Refusal: 500 FlowTicketIssueFailed when the ticket cannot be issued.
type AuthCodeRequestHandler struct {
	Provider    *Provider
	RedirectURI string
	Flow        *authn.FlowManager
}

func (h *AuthCodeRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ticket, err := h.Flow.IssueTicket(w)
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.FlowTicketIssueFailed.WithCause(err))
		return
	}
	http.Redirect(w, r, h.Provider.AuthCodeURL(ticket, h.RedirectURI), http.StatusFound)
}
