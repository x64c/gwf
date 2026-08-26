package framework

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web"
	"github.com/x64c/gwf/gw/web/fwupstream"
)

// PrepareFWUpstream loads the app's single `.fwupstream-web.json` and builds the
// upstream subsystem hub on c.FWUpstream:
//
//   - clients: per-Client config keyed by Client id; an *fwupstream.Client
//     is built per id present in the fwClients arg and stored under
//     c.FWUpstream.Clients[id]. Each gets a shallow clone of c.BaseHttpClient so
//     per-client transport tweaks don't bleed across.
//   - token_cipher (optional): the at-rest keyring for upstream OAuth tokens
//     ({active, keys{}} — see security.KeyringConf). Present iff the downstream
//     stores upstream tokens; absent for JWKS-only upstreams. Each key's
//     "enckey" is base64-encoded 32 random master bytes (openssl rand -base64
//     32); the working key is derived per purpose, so this keyring may share
//     master material with the cookie one without their values ever crossing.
//
// The Hub is self-contained — it can be used without sessions (call
// c.FWUpstream.Clients[...] for outbound requests, or c.FWUpstream's token I/O with
// your own row key). The session managers merely reference c.FWUpstream and
// delegate token I/O to it, so for apps that store upstream tokens this must run
// BEFORE PrepareCookieSessions / PrepareBearerSessions (they copy c.FWUpstream at
// construction).
//
// The fwClients arg's keys are the Client ids to construct (each must have a
// matching entry under "clients"); the values are per-Client setter callbacks
// that run after each Client is built, used to register typed refresh
// sideloaders via session packages' typed setters (e.g.
// bearer.SetUserRefreshSideloader, cookie.SetUserRefreshSideloader). Pass nil
// as the value if a particular Client needs no sideloaders (e.g. used only
// for JWKS fetch).
//
// Example:
//
//	app.PrepareFWUpstream(map[string]func(*fwupstream.Client){
//	    "main": func(c *fwupstream.Client) {
//	        bearer.SetUserRefreshSideloader[string](c, func(ctx context.Context, sd *bearer.UserSessionData[string]) map[string]any {
//	            return map[string]any{"uid": sd.UIDStr}
//	        })
//	    },
//	})
func (c *Core) PrepareFWUpstream(fwClients map[string]func(*fwupstream.Client)) error {
	if c.BaseHttpClient == nil {
		return errors.New("base http client is not ready")
	}
	if c.MainKVDB == nil {
		return errors.New("main kvdb not ready")
	}
	if len(fwClients) == 0 {
		return errors.New("no id provided")
	}

	confPath := filepath.Join(c.AppRoot, "config", ".fwupstream-web.json")
	confBytes, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}
	conf := &fwupstream.Conf{}
	if err := json.Unmarshal(confBytes, conf); err != nil {
		return err
	}

	hub := &fwupstream.Hub{
		Clients: make(map[string]*fwupstream.Client, len(fwClients)),
		KVDB:    c.MainKVDB,
	}

	// token_cipher is optional — present only when the app stores upstream tokens.
	// Construction validates the whole keyring (active among keys, key ids,
	// algs, key material) — misconfiguration is a boot failure here, never a
	// per-request surprise.
	if conf.TokenCipher != nil {
		cipher, err := security.NewKeyringCipher(conf.TokenCipher, fwupstream.TokenCipherPurpose)
		if err != nil {
			return fmt.Errorf("upstream token cipher: %v", err)
		}
		hub.TokenCipher = cipher
	}

	for id, setRefreshSideloaders := range fwClients {
		clientConf, ok := conf.Clients[id]
		if !ok {
			return fmt.Errorf("upstream client %q has no entry under .fwupstream-web.json \"clients\"", id)
		}
		fwc := &fwupstream.Client{
			Client: web.ShallowCloneClient(c.BaseHttpClient),
			ID:     id,
			Conf:   clientConf,
		}
		if setRefreshSideloaders != nil {
			setRefreshSideloaders(fwc)
		}
		hub.Clients[id] = fwc
	}

	c.FWUpstream = hub
	return nil
}
