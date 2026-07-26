package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/security"
)

type JwksDeleteOldRsakeys struct {
	AppProvider framework.AppProviderFunc
}

func (*JwksDeleteOldRsakeys) GroupName() string { return "jwks" }
func (h *JwksDeleteOldRsakeys) Command() string { return "jwks-delete-old-rsakeys" }
func (h *JwksDeleteOldRsakeys) Desc() string    { return "Delete Old RSA key pair files" }
func (h *JwksDeleteOldRsakeys) Usage() string   { return h.Command() }

func (h *JwksDeleteOldRsakeys) HandleCommand(_ []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	kid, ok, err := appCore.ActiveKid(appCore.RootCtx)
	if err != nil {
		return fmt.Errorf("failed to get current kid: %v", err)
	}
	if !ok {
		return fmt.Errorf("kid not found")
	}
	if err := security.DeleteOldRSAKeys(
		appCore.JwksServiceConf.PrivateKeyDir,
		appCore.JwksServiceConf.PublicKeyDir,
		kid,
	); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "Old RSA key pair files deleted")
	return nil
}
