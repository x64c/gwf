package uds

import (
	"fmt"
	"io/fs"
	"strconv"
)

// Conf is the UDS service's configuration, loaded from config/.uds.json by
// framework.PrepareUDSService. Every field is REQUIRED-explicit: this is the
// operator surface, and the framework refuses boot rather than pick one of
// its values on the deployment's behalf.
type Conf struct {
	// SocketPath is where the socket file is created. REQUIRED. Placement is
	// the deployment's choice; note that DELETION rights on a Unix file live
	// on its directory, not the file — pick a directory writable only by the
	// app's user, or one with the sticky bit if it must be shared.
	SocketPath string `json:"socket_path"`

	// SocketMode is the socket file's permission mode, an octal string such
	// as "0660" (owner+group; connecting requires WRITE permission on the
	// socket). REQUIRED; octal, permission bits only (up to "0777") — the
	// leading zero is optional, the base is always 8. "0" is legal and means
	// root-only: root passes permission checks regardless. Unset, the
	// framework used to hardcode 0660 — the deployment states its mode here.
	SocketMode string `json:"socket_mode"`

	// MaxLineBytes caps ONE command line read from a client connection.
	// REQUIRED (bytes > 0). Bounds the buffer a client can make the server
	// grow; an over-cap line gets an error line back and the connection is
	// closed. Unset, the service used to hardcode 1 MiB (1048576).
	MaxLineBytes int `json:"max_line_bytes"`
}

// Validate checks this conf's own invariants.
func (c Conf) Validate() error {
	if c.SocketPath == "" {
		return fmt.Errorf("socket_path must be set in .uds.json")
	}
	if c.SocketMode == "" {
		return fmt.Errorf(`socket_mode must be set (octal string, e.g. "0660") in .uds.json — the framework used to hardcode 0660, the deployment states its mode`)
	}
	mode, err := strconv.ParseUint(c.SocketMode, 8, 32)
	if err != nil {
		return fmt.Errorf(`socket_mode %q in .uds.json is not an octal mode (e.g. "0660"): %w`, c.SocketMode, err)
	}
	if mode > 0o777 {
		return fmt.Errorf(`socket_mode %q in .uds.json exceeds permission bits (max "0777"): special bits have no effect on a socket`, c.SocketMode)
	}
	if c.MaxLineBytes <= 0 {
		return fmt.Errorf("max_line_bytes must be set (bytes > 0) in .uds.json: got %d — the service used to hardcode 1 MiB (1048576)", c.MaxLineBytes)
	}
	return nil
}

// Mode is the fs.FileMode socket_mode states. Validate has already rejected
// malformed values, so this cannot fail.
func (c Conf) Mode() fs.FileMode {
	mode, _ := strconv.ParseUint(c.SocketMode, 8, 32)
	return fs.FileMode(mode)
}
