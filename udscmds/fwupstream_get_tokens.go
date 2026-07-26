package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
)

type FwupstreamGetTokens struct {
	AppProvider framework.AppProviderFunc
}

func (h *FwupstreamGetTokens) GroupName() string {
	return "fwupstream"
}

func (h *FwupstreamGetTokens) Command() string {
	return "fwupstream-get-tokens"
}

func (h *FwupstreamGetTokens) Desc() string {
	return "Print the decrypted upstream tokens stored on a session row"
}

func (h *FwupstreamGetTokens) Usage() string {
	return h.Command() + " session-row-key client-id"
}

func (h *FwupstreamGetTokens) HandleCommand(args []string, w io.Writer) error {
	argLen := len(args)
	if argLen != 2 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	rowKey := args[0]
	clientID := args[1]
	// clientID is deliberately not checked against Hub.Clients: the field name
	// derives from the string, so leftover fields of since-removed upstreams
	// stay reachable.
	appCore := h.AppProvider().AppCore()
	hub := appCore.FWUpstream
	if hub == nil {
		return fmt.Errorf("upstream subsystem not configured (PrepareFWUpstream not called)")
	}
	if hub.TokenCipher == nil {
		return errs.UpstreamTokenCipherNotSet
	}
	ctx := appCore.RootCtx
	accessTkn, resErr := hub.FetchAccessToken(ctx, rowKey, clientID)
	_, _ = fmt.Fprintf(w, "access: %s\n", tokenLine(accessTkn, resErr))
	refreshTkn, resErr := hub.FetchRefreshToken(ctx, rowKey, clientID)
	_, _ = fmt.Fprintf(w, "refresh: %s\n", tokenLine(refreshTkn, resErr))
	return nil
}

// tokenLine renders a fetched token for display: the plaintext, "—" when the
// session row has no such field, or the fetch error.
func tokenLine(tkn string, resErr *errs.Error) string {
	if resErr == nil {
		return tkn
	}
	if resErr.IsSameCode(errs.UpstreamAccessTokenNotFound) || resErr.IsSameCode(errs.UpstreamRefreshTokenNotFound) {
		return "—"
	}
	return "error: " + resErr.Error()
}
