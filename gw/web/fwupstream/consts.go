package fwupstream

// Field-name schema for upstream tokens stored as fields on a session's
// base hash row. Used by per-session-flavor Managers (cookie, bearer).
// The clientID in field names is the internal client id — the Clients map
// key from .fwupstream-web.json (Client.ID, e.g. "main") — NOT ClientConf.ClientID
// (the OAuth client identifier sent to the upstream).
const (
	accessTknFieldPrefix  = "up:a:"
	refreshTknFieldPrefix = "up:r:"
)
