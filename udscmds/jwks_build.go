package udscmds

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/security"
)

type JwksBuild struct {
	AppProvider framework.AppProviderFunc
}

func (*JwksBuild) GroupName() string { return "jwks" }
func (h *JwksBuild) Command() string { return "jwks-build" }
func (h *JwksBuild) Desc() string    { return "Rebuild jwks.json file from key files" }
func (h *JwksBuild) Usage() string   { return h.Command() }

func (h *JwksBuild) HandleCommand(_ []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	jwksPath := filepath.Join(appCore.AppRoot, "gen", "jwks.json")
	if err := security.BuildJWKSFile(appCore.JwksServiceConf.PublicKeyDir, jwksPath); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "jwks.json created")
	return nil
}
