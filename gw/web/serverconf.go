package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/web/requests"
)

// ServerConf is the HTTP server's own configuration, loaded from
// config/.web-server.json by framework.LoadWebServerConf.
//
// Every field is REQUIRED-explicit. Go's zero value for each of these means
// "no limit", or an address nobody chose — the very defect they close — so the
// framework rejects boot rather than decide on the operator's behalf. The
// server is assumed to stand alone: behind a proxy these deadlines are defence
// in depth, directly exposed they are the only defence, and the conf takes no
// position on which deployment it is in.
type ServerConf struct {
	Listen string `json:"listen"` // REQUIRED. Address the server binds, "ip:port".
	Host   string `json:"host"`   // REQUIRED. The app's canonical public origin, scheme included (e.g. "https://example.com").

	ReadHeaderTimeoutSecs int `json:"read_header_timeout_secs"` // REQUIRED (> 0, <= read_timeout_secs). Deadline for reading the request headers. Bounds header-stall (slowloris) connections, which hold a socket without ever reaching a handler.
	ReadTimeoutSecs       int `json:"read_timeout_secs"`        // REQUIRED (>= read_header_timeout_secs). Deadline for reading the WHOLE request, headers plus body. Must exceed the slowest legitimate upload on the slowest supported connection.
	WriteTimeoutSecs      int `json:"write_timeout_secs"`       // REQUIRED (> 0). Deadline from end-of-header-read to the last response byte, so it bounds HANDLER time too. Must exceed the slowest legitimate response (report/PDF generation, long polls, streamed output).
	IdleTimeoutSecs       int `json:"idle_timeout_secs"`        // REQUIRED (> 0). How long an idle keep-alive connection is kept open between requests. Shorter reclaims sockets from idle peers sooner; longer avoids a connection-pooling peer (browser, SDK, proxy upstream) racing to reuse a connection this server just closed.
	DrainTimeoutSecs      int `json:"drain_timeout_secs"`       // REQUIRED (> 0, < core svc_terminate_timeout_secs). Graceful-drain window for Server.Shutdown on stop — the grace period in-flight requests get to finish. A request may outlive it (write_timeout is the longer bound); when it closes, request contexts are canceled so handlers can unwind instead of being hard-killed.
	MaxHeaderBytes        int `json:"max_header_bytes"`         // REQUIRED (> 0). Upper bound on the request line plus all headers, per request (net/http enforces it with a small internal read slack, ~4 KiB). Unset, Go silently applies its own 1 MiB — a limit nobody chose and no conf states; the deployment states its number here (Go's 1048576 is a reasonable one).

	// TrustedProxyCIDRs lists the proxies allowed to state who the caller is.
	// Optional; absent or empty means trust nothing, and the client address is
	// then the connection's peer — honest, but behind an undeclared proxy every
	// request resolves to that proxy. Entries are CIDRs with the prefix length
	// written out ("127.0.0.1/32", "10.0.0.0/24", "::1/128"); a bare address is
	// rejected at boot rather than assumed. See web.ClientIPResolver for why the
	// rightmost forwarded entry is the trustworthy one.
	TrustedProxyCIDRs []string `json:"trusted_proxy_cidrs"`
}

// Validate checks this conf's own invariants. Cross-layer relations — the
// drain window against the core shutdown budget — belong to the caller, the
// only place where both confs are settled.
func (c ServerConf) Validate() error {
	if c.Listen == "" {
		return fmt.Errorf(`listen must be set ("ip:port") in .web-server.json`)
	}
	if c.Host == "" {
		return fmt.Errorf(`host must be set (e.g. "https://example.com") in .web-server.json`)
	}
	if c.ReadHeaderTimeoutSecs <= 0 {
		return fmt.Errorf("read_header_timeout_secs must be set (seconds > 0) in .web-server.json: got %d", c.ReadHeaderTimeoutSecs)
	}
	if c.ReadTimeoutSecs <= 0 {
		return fmt.Errorf("read_timeout_secs must be set (seconds > 0) in .web-server.json: got %d", c.ReadTimeoutSecs)
	}
	if c.WriteTimeoutSecs <= 0 {
		return fmt.Errorf("write_timeout_secs must be set (seconds > 0) in .web-server.json: got %d", c.WriteTimeoutSecs)
	}
	if c.IdleTimeoutSecs <= 0 {
		return fmt.Errorf("idle_timeout_secs must be set (seconds > 0) in .web-server.json: got %d", c.IdleTimeoutSecs)
	}
	if c.DrainTimeoutSecs <= 0 {
		return fmt.Errorf("drain_timeout_secs must be set (seconds > 0) in .web-server.json: got %d", c.DrainTimeoutSecs)
	}
	if c.MaxHeaderBytes <= 0 {
		return fmt.Errorf("max_header_bytes must be set (bytes > 0) in .web-server.json: got %d — unset, Go would silently apply its own 1 MiB", c.MaxHeaderBytes)
	}
	if c.ReadHeaderTimeoutSecs > c.ReadTimeoutSecs {
		return fmt.Errorf("read_header_timeout_secs (%d) must not exceed read_timeout_secs (%d): the whole-request deadline would fire first, leaving the header deadline dead", c.ReadHeaderTimeoutSecs, c.ReadTimeoutSecs)
	}
	if _, err := requests.NewClientIPResolver(c.TrustedProxyCIDRs); err != nil {
		return fmt.Errorf("trusted_proxy_cidrs in .web-server.json: %w (write the prefix length, e.g. 127.0.0.1/32)", err)
	}
	return nil
}

// ClientIPResolver builds the resolver this conf describes. Validate has
// already rejected malformed entries, so this cannot fail.
func (c ServerConf) ClientIPResolver() requests.ClientIPResolver {
	rs, _ := requests.NewClientIPResolver(c.TrustedProxyCIDRs)
	return rs
}

// newHTTPServer builds the *http.Server this conf describes. One place maps
// conf fields to server fields; the service only runs what it is given.
//
// baseCtx becomes the parent of every request context. Its lifetime — not any
// conf value — decides when in-flight handlers are told to give up.
func (c ServerConf) newHTTPServer(addr string, handler http.Handler, baseCtx context.Context) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
		ReadHeaderTimeout: secs(c.ReadHeaderTimeoutSecs),
		ReadTimeout:       secs(c.ReadTimeoutSecs),
		WriteTimeout:      secs(c.WriteTimeoutSecs),
		IdleTimeout:       secs(c.IdleTimeoutSecs),
		MaxHeaderBytes:    c.MaxHeaderBytes,
	}
}

// drainTimeout is the Server.Shutdown budget this conf describes.
func (c ServerConf) drainTimeout() time.Duration {
	return secs(c.DrainTimeoutSecs)
}

func secs(n int) time.Duration {
	return time.Duration(n) * time.Second
}
