// Package authn holds the framework's authentication seam: the types every
// authentication method produces, and the flow ticket for browser-mediated
// multi-request flows.
//
// The package covers identity establishment only — proving who a caller is.
// What an app does with an established identity (which session flavor, what
// response shape) is the app's composition, built from the session packages.
package authn

// Method identifies the authentication method that established an identity.
// Each method implementation declares its own Method constant; the framework
// declares none and never branches on the value.
type Method string

// VerifiedIdentity is what every authentication method produces on success:
// an external identity, verified, not yet mapped to a local user.
//
// Subject is the method's one canonical external identifier — which field of
// the external identity it carries is defined by each method (an email
// address, a token subject, a username). Claims carries the method's extra
// attributes; keys and value types are method-defined.
type VerifiedIdentity struct {
	Method  Method
	Subject string
	Claims  map[string]any
}
