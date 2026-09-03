package framework

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/coord"
)

// NewCore builds a Core from <appRoot>/config/.core.json, read once, here.
// Two of its keys are the app's IDENTITY and land in unexported fields, so
// neither app code between Prepares nor any later decoding can change what
// the app IS after construction:
//
//	"app_name"    the app's name: the namespace every shared record keys
//	              under — rows, locks, sessions, tickets. Required.
//	"coord_mode"  "inproc" or "crossproc": where the state the app's
//	              instances must agree on lives (see coord.Mode). Required.
//
// The rest of the file is Core's boot wiring, decoded into its exported
// fields; today that is "svc_terminate_timeout_secs" (required, > 0), the
// per-service terminate budget. Everything
// else on Core is set by the Prepare* calls.
//
// Identity is a deployment fact, not a build fact: one binary runs under any
// number of confs, and what the two values mean together is this —
//
//   - Processes running under "crossproc" with the same app_name against
//     the same shared store are ONE distributed app. They share every record,
//     so they must be built from the same code.
//   - A different app_name is a different app, even from the same binary:
//     its own keyspace, its own locks, its own sessions, side by side with
//     the others on the same store.
//   - "inproc" is one instance. Two "inproc" processes with the same
//     app_name on one store are not an app; they collide (see coord.InProc).
//
// Both identity keys are validated at the gate: an app with no name cannot
// namespace its shared records, and a mode is a choice, never a default.
func NewCore(appRoot string) (*Core, error) {
	confPath := filepath.Join(appRoot, "config", ".core.json")
	confBytes, err := os.ReadFile(confPath)
	if err != nil {
		return nil, fmt.Errorf("NewCore: %w", err)
	}

	// The identity keys, decoded on their own so that they land in the
	// unexported fields — the decode into Core below cannot reach those.
	var identity struct {
		AppName   string `json:"app_name"`
		CoordMode string `json:"coord_mode"`
	}
	if err := json.Unmarshal(confBytes, &identity); err != nil {
		return nil, fmt.Errorf("NewCore: %s: %w", confPath, err)
	}
	if identity.AppName == "" {
		return nil, errors.New("NewCore: \"app_name\" required in " + confPath)
	}
	mode, err := coord.ParseMode(identity.CoordMode)
	if err != nil {
		return nil, fmt.Errorf("NewCore: \"coord_mode\" in %s: %w", confPath, err)
	}

	c := &Core{appName: identity.AppName, coordMode: mode, AppRoot: appRoot}
	if err := json.Unmarshal(confBytes, c); err != nil {
		return nil, fmt.Errorf("NewCore: %s: %w", confPath, err)
	}
	if c.SvcTerminateTimeoutSecs <= 0 {
		return nil, fmt.Errorf("NewCore: \"svc_terminate_timeout_secs\" must be set (seconds > 0) in %s: got %d", confPath, c.SvcTerminateTimeoutSecs)
	}
	return c, nil
}

// AppName is the app's fleet identity: the namespace its shared records key
// under, and the name every instance of this app shares — read once from
// .core.json by NewCore.
func (c *Core) AppName() string { return c.appName }

// CoordMode is the app's coordination identity: the implementation family
// every Prepare* seats its coordination claims from.
func (c *Core) CoordMode() coord.Mode { return c.coordMode }
