package udscmds

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/security"
)

type JwksKidSet struct {
	AppProvider framework.AppProviderFunc
}

func (*JwksKidSet) GroupName() string { return "jwks" }
func (*JwksKidSet) Command() string   { return "jwks-kid-set" }
func (*JwksKidSet) Desc() string {
	return "Set the active KID in KVDB. If kid omitted, picks the newest private PEM by mtime."
}
func (h *JwksKidSet) Usage() string { return h.Command() + " [kid]" }

func (h *JwksKidSet) HandleCommand(args []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	privKeyDir := appCore.JwksServiceConf.PrivateKeyDir

	var kid string
	if len(args) > 0 && args[0] != "" {
		kid = args[0]
		privPath := filepath.Join(privKeyDir, kid+"_private.pem")
		if _, err := os.Stat(privPath); err != nil {
			return fmt.Errorf("private key not found for kid %q: %v", kid, err)
		}
	} else {
		latest, err := security.FindLatestKidByMtime(privKeyDir)
		if err != nil {
			return err
		}
		kid = latest
	}

	curr, found, err := appCore.ActiveKid(appCore.RootCtx)
	if err != nil {
		return fmt.Errorf("failed to read kid from kvdb: %v", err)
	}
	if found && curr == kid {
		_, _ = fmt.Fprintf(w, "kid unchanged: %s\n", kid)
		return nil
	}
	if err := appCore.SetActiveKid(appCore.RootCtx, kid); err != nil {
		return fmt.Errorf("failed to set kid in kvdb: %v", err)
	}
	_, _ = fmt.Fprintf(w, "kid set: %s\n", kid)
	return nil
}
