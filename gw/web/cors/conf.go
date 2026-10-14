package cors

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// Conf is one access control key's deployment half of a Policy: which origins may read, and how long a browser may
// reuse a preflight answer.
type Conf struct {
	AllowOrigins            []string `json:"allow_origins"`               // REQUIRED. Exact origins — scheme://host[:port], lowercase, no path, no default port — or ["*"] alone: any origin.
	AccessControlMaxAgeSecs int      `json:"access_control_max_age_secs"` // REQUIRED (> 0). How long a browser may reuse a preflight answer (Access-Control-Max-Age). Browsers cap it.
}

// Validate refuses a Conf with a value that is unset or cannot describe what it names: no origin, "*" beside an
// origin, an origin a browser would never send, the same origin twice, no max age.
func (c Conf) Validate() error {
	if len(c.AllowOrigins) == 0 {
		return fmt.Errorf("allow_origins: no origin")
	}
	if slices.Contains(c.AllowOrigins, "*") {
		if len(c.AllowOrigins) > 1 {
			return fmt.Errorf(`allow_origins: "*" (any origin) stands alone`)
		}
	} else {
		seen := make(map[string]struct{}, len(c.AllowOrigins))
		for _, o := range c.AllowOrigins {
			if err := validateOrigin(o); err != nil {
				return fmt.Errorf("allow_origins: %w", err)
			}
			if _, dup := seen[o]; dup {
				return fmt.Errorf("allow_origins: %q twice", o)
			}
			seen[o] = struct{}{}
		}
	}
	if c.AccessControlMaxAgeSecs <= 0 {
		return fmt.Errorf("access_control_max_age_secs must be set (seconds > 0): got %d", c.AccessControlMaxAgeSecs)
	}
	return nil
}

// validateOrigin refuses anything a browser would never send as an Origin: a browser serializes an origin as
// scheme://host[:port] — lowercase, no path, no default port — so any other spelling would never match.
func validateOrigin(o string) error {
	u, err := url.Parse(o)
	if err != nil {
		return fmt.Errorf("%q is not an origin: %w", o, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("%q is not an origin: want scheme://host[:port] with scheme http or https and nothing after", o)
	}
	if o != strings.ToLower(u.Scheme+"://"+u.Host) {
		return fmt.Errorf("%q is not an origin as a browser sends it: lowercase it", o)
	}
	if port := u.Port(); (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		return fmt.Errorf("%q names its scheme's default port, which a browser leaves out: drop it", o)
	}
	return nil
}
