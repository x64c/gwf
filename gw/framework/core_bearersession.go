package framework

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/web/session/bearer"
)

// Valid identity binding labels recognized at boot. Unknown labels in a
// group's binds fail validation.
var validBearerBinds = map[string]struct{}{
	"client": {},
	"user":   {},
}

// PrepareBearerSessions reads .bearer-session.json, parses the group→clients
// tree, validates the structure, populates per-entry back-refs (Name, Group),
// and builds the by-ID ClientConfs lookup map. The manager is attached to
// SessionService.BearerSessionManager.
//
// Prerequisite: MainKVDB, SessionService.
//
// Pass useFWUpstream=true if bearer sessions store upstream tokens; the manager
// then takes c.FWUpstream, and this fails at boot if it isn't prepared yet
// (call PrepareFWUpstream first). Pass false if bearer sessions have no upstream.
func (c *Core) PrepareBearerSessions(useFWUpstream bool) error {
	if c.SessionService == nil {
		return errors.New("session service not ready")
	}
	if c.MainKVDB == nil {
		return errors.New("main kvdb not ready")
	}

	confFilePath := filepath.Join(c.AppRoot, "config", ".bearer-session.json")
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		return err
	}

	var groups map[string]*bearer.SessionGroupConf
	if err := json.Unmarshal(confBytes, &groups); err != nil {
		return err
	}

	clientNames := make(map[string]string) // client name → group it lives in
	clientIDs := make(map[string]string)   // client id → group it lives in
	clientConfs := make(map[string]*bearer.SessionClientConf)

	for groupName, group := range groups {
		if groupName == "" {
			return errors.New("group name must be non-empty")
		}

		// per-client: dup checks + back-refs + by-ID map build
		group.Name = groupName
		for clientName, client := range group.Clients {
			if otherGroup, dup := clientNames[clientName]; dup {
				return fmt.Errorf("duplicate client name %q in groups %q and %q", clientName, otherGroup, groupName)
			}
			clientNames[clientName] = groupName
			if otherGroup, dup := clientIDs[client.ID]; dup {
				return fmt.Errorf("duplicate client id %q in groups %q and %q", client.ID, otherGroup, groupName)
			}
			clientIDs[client.ID] = groupName

			client.Name = clientName
			client.Group = group
			clientConfs[client.ID] = client
		}

		// binds: non-empty + recognized + includes "client"
		if len(group.Binds) == 0 {
			return fmt.Errorf("group %q: binds must be non-empty", groupName)
		}
		bindsSet := make(map[string]struct{}, len(group.Binds))
		hasClient := false
		for _, b := range group.Binds {
			if _, ok := validBearerBinds[b]; !ok {
				return fmt.Errorf("group %q: unknown binding label %q", groupName, b)
			}
			if b == "client" {
				hasClient = true
			}
			bindsSet[b] = struct{}{}
		}
		if !hasClient {
			return fmt.Errorf("group %q: binds must include %q", groupName, "client")
		}

		// clients map non-empty
		if len(group.Clients) == 0 {
			return fmt.Errorf("group %q: clients map must be non-empty", groupName)
		}

		// TTLs > 0
		if group.AccessTTL <= 0 {
			return fmt.Errorf("group %q: access_ttl must be > 0", groupName)
		}
		if group.RefreshTTL <= 0 {
			return fmt.Errorf("group %q: refresh_ttl must be > 0", groupName)
		}
		if group.RefreshChainTTL <= 0 {
			return fmt.Errorf("group %q: refresh_chain_ttl must be > 0", groupName)
		}
		if group.RefreshTTL > group.RefreshChainTTL {
			return fmt.Errorf("group %q: refresh_ttl (%d) must be <= refresh_chain_ttl (%d)", groupName, group.RefreshTTL, group.RefreshChainTTL)
		}

		// cap.by axes must be subset of binds (only when cap is meaningfully enabled)
		if group.Cap != nil && group.Cap.Max > 0 {
			if len(group.Cap.By) == 0 {
				return fmt.Errorf("group %q: cap.by must be non-empty when cap.max > 0", groupName)
			}
			for _, axis := range group.Cap.By {
				if _, ok := bindsSet[axis]; !ok {
					return fmt.Errorf("group %q: cap.by axis %q not in binds", groupName, axis)
				}
			}
		}
	}

	mgr := &bearer.SessionManager{
		ClientConfs:   clientConfs,
		GroupConfs:    groups,
		AppName:       c.AppName,
		KVDB:          c.MainKVDB,
		SessionLocks:  c.SessionService.SessionLocks,
		ParentService: c.SessionService,
	}

	// Wire the upstream subsystem only when the app declares bearer sessions
	// store upstream tokens — and verify it was prepared, failing loud at boot.
	if useFWUpstream {
		if c.FWUpstream == nil {
			return errors.New("bearer sessions: useFWUpstream=true but FWUpstream not prepared — call PrepareFWUpstream first")
		}
		mgr.FWUpstream = c.FWUpstream
	}

	c.SessionService.BearerSessionManager = mgr
	mgr.Enable() // bearer protocol wired → serving
	return nil
}
