package udscmds

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/security"
)

type JwksRotate struct {
	AppProvider framework.AppProviderFunc
}

func (h *JwksRotate) GroupName() string { return "jwks" }
func (h *JwksRotate) Command() string   { return "jwks-rotate" }
func (h *JwksRotate) Desc() string      { return "Rotate RSA key pair and build jwks" }
func (h *JwksRotate) Usage() string     { return h.Command() }

func (h *JwksRotate) HandleCommand(_ []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	kid, pubKey, err := security.GenerateAndSaveRSAKey(
		appCore.JwksServiceConf.PrivateKeyDir,
		appCore.JwksServiceConf.PublicKeyDir,
	)
	if err != nil {
		return err
	}
	if err = appCore.SetActiveKid(appCore.RootCtx, kid); err != nil {
		return fmt.Errorf("failed to save current key: %v", err)
	}
	jwksPath := filepath.Join(appCore.AppRoot, "gen", "jwks.json")
	if err = security.BuildJWKSFile(appCore.JwksServiceConf.PublicKeyDir, jwksPath); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "jwks.json created\npublic key: %v\nkid: %s\n", pubKey, kid)
	return nil
}
