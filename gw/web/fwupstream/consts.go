package fwupstream

// Field-name schema for upstream tokens stored as fields on a session's
// base hash row. Used by per-session-flavor Managers (cookie, bearer).
// ClientID is the upstream OAuth client identifier.
const (
	accessTknFieldPrefix  = "up:a:"
	refreshTknFieldPrefix = "up:r:"
)
