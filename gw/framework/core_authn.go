package framework

import (
	"errors"
	"fmt"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/security"
)

// PrepareAuthnFlowManager prepares the authn flow-ticket manager. Its cipher
// is derived from the cookie keyring under authn.FlowCipherPurpose — a flow
// ticket is a cookie and lives under cookie custody. Prerequisite:
// PrepareCookieSessions (the keyring is read there).
func (c *Core) PrepareAuthnFlowManager() (*authn.FlowManager, error) {
	if c.sessionService == nil || c.sessionService.CookieSessionManager == nil {
		return nil, errors.New("authn flow: cookie sessions not prepared — call PrepareCookieSessions first")
	}
	keyring := c.sessionService.CookieSessionManager.Conf.Keyring
	if keyring == nil {
		return nil, errors.New("authn flow: no \"keyring\" in .web-cookie-session.json")
	}
	cipher, err := security.NewKeyringCipher(keyring, authn.FlowCipherPurpose)
	if err != nil {
		return nil, fmt.Errorf("authn flow cipher: %v", err)
	}
	return &authn.FlowManager{AppName: c.AppName, Cipher: cipher}, nil
}
