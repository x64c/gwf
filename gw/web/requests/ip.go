package requests

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIPResolver derives the caller's address from a request. It is built
// from the deployment's trusted proxies, because that list is what makes
// forwarding headers meaningful: `X-Forwarded-For` and `X-Real-IP` are written
// by whoever sends the request, so a value is worth reading only when a party
// we trust put it there.
//
// Resolution walks the forwarded chain from the RIGHT. The rightmost entry was
// appended by the proxy nearest to us; the leftmost is whatever the caller
// typed. Skipping trusted hops right-to-left and stopping at the first untrusted
// entry therefore returns a value a trusted party recorded, and never reaches
// anything the caller controls. Trust too little and the answer is coarse (a
// proxy's address); trust the wrong thing and it is forged — so the list should
// name the narrowest prefixes that are true.
//
// With no trusted proxies the headers are ignored and the answer is the peer
// address of the connection — the one value a caller cannot assert.
type ClientIPResolver struct {
	trusted []netip.Prefix
}

// NewClientIPResolver builds a resolver from CIDR strings ("127.0.0.1/32",
// "10.0.0.0/24", "::1/128"). The prefix length is required — a bare address is
// rejected rather than assumed, since a trust list is the last place to guess.
// An empty list is legal and means: trust nothing, use the peer address.
func NewClientIPResolver(cidrs []string) (ClientIPResolver, error) {
	var rs ClientIPResolver
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(strings.TrimSpace(c))
		if err != nil {
			return ClientIPResolver{}, err
		}
		rs.trusted = append(rs.trusted, p.Masked())
	}
	return rs, nil
}

// TrustedCount reports how many proxy prefixes this resolver trusts.
func (rs ClientIPResolver) TrustedCount() int {
	return len(rs.trusted)
}

// ClientIP returns the caller's address for this request.
func (rs ClientIPResolver) ClientIP(r *http.Request) string {
	peer := peerAddr(r)
	if len(rs.trusted) == 0 {
		return peer
	}

	// The peer itself must be trusted, or the chain is being entered from the
	// side — a caller reaching this server directly may claim nothing.
	peerIP, err := netip.ParseAddr(peer)
	if err != nil || !rs.isTrusted(peerIP) {
		return peer
	}

	// Right to left: skip trusted hops, stop at the first address a trusted
	// proxy recorded for us.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
			if err != nil {
				continue // not an address: no identity here, keep walking
			}
			if !rs.isTrusted(ip) {
				return ip.Unmap().String()
			}
		}
	}

	// Every hop in the chain was trusted (or there was no chain): X-Real-IP is
	// the trusted peer's own statement of who connected to it.
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if ip, err := netip.ParseAddr(xri); err == nil {
			return ip.Unmap().String()
		}
	}
	return peer
}

func (rs ClientIPResolver) isTrusted(ip netip.Addr) bool {
	ip = ip.Unmap()
	for _, p := range rs.trusted {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// peerAddr is the address of the connection itself.
func peerAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Unmap().String()
	}
	return host
}
