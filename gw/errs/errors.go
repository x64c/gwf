package errs

// Pre-built errors with default messages.
// For errors that need runtime data in the message, use WithDetail:
//   errs.PermissionDenied.WithDetail("some detail")
//
// Framework-level logic error codes (1000-1999).
// App-level codes use 2000+ in their own `reasons` package.
// Code 0 = no logic code (client falls back to HTTP status).

var (
	// Requests

	ContentLengthTooLarge = &Error{Name: "ContentLengthTooLarge", Code: 1000, Message: "Content-Length header is too large"} // declared body would exceed limit — caught upfront from the Content-Length header
	RequestBodyTooLarge   = &Error{Name: "RequestBodyTooLarge", Code: 1001, Message: "request body too large"}               // actual body bytes exceeded limit during streaming read
	OriginNotAllowed      = &Error{Name: "OriginNotAllowed", Code: 1010, Message: "request origin not allowed"}              // Origin header missing, or not in the configured allowlist
	ServiceUnavailable    = &Error{Name: "ServiceUnavailable", Code: 1020, Message: "service unavailable"}                   // a service the endpoint needs is not admitted for use right now (stopped, terminating, or never wired)
	ActionLocked          = &Error{Name: "ActionLocked", Code: 1030, Message: "action locked by another request"}            // a named action lock the endpoint needs is held; retry later
	InternalError         = &Error{Name: "InternalError", Code: 1050, Message: "internal server error"}

	// Session

	// ---- Session Service

	SessionServiceUnavailable = &Error{Name: "SessionServiceUnavailable", Code: 1100, Message: "session service unavailable"}

	// ---- Bearer Session

	AccessTokenNotFound          = &Error{Name: "AccessTokenNotFound", Code: 1110, Message: "access token not found"}
	RefreshTokenNotFound         = &Error{Name: "RefreshTokenNotFound", Code: 1111, Message: "refresh token not found"}
	InvalidAccessToken           = &Error{Name: "InvalidAccessToken", Code: 1112, Message: "invalid access token"}
	InvalidRefreshToken          = &Error{Name: "InvalidRefreshToken", Code: 1113, Message: "invalid refresh token"}
	BearerSessionShapeMismatch   = &Error{Name: "BearerSessionShapeMismatch", Code: 1114, Message: "bearer session shape mismatch"}      // session's binds shape doesn't fit endpoint (e.g. userless on user-bound endpoint)
	BearerSessionGroupNotAllowed = &Error{Name: "BearerSessionGroupNotAllowed", Code: 1115, Message: "bearer session group not allowed"} // session's group isn't in the endpoint's allowlist
	BearerClientNotFound         = &Error{Name: "BearerClientNotFound", Code: 1116, Message: "bearer client not found"}                  // Client-Id missing in request OR unknown to the bearer client registry
	BearerSessionNotFound        = &Error{Name: "BearerSessionNotFound", Code: 1117, Message: "bearer session not found"}                // the session row a token resolves to is gone; counterpart of CookieSessionNotFound
	BearerUserClaimInvalid       = &Error{Name: "BearerUserClaimInvalid", Code: 1118, Message: "bearer user claim invalid"}              // user token exchange: the identity's user claim is absent or names no user
	BearerSessionNotOwned        = &Error{Name: "BearerSessionNotOwned", Code: 1119, Message: "bearer session not owned by client"}      // the session belongs to another client than the caller

	// ---- Cookie Session

	CookieNotFound        = &Error{Name: "CookieNotFound", Code: 1120, Message: "cookie not found"}
	InvalidCookie         = &Error{Name: "InvalidCookie", Code: 1121, Message: "invalid cookie"}
	CookieSessionNotFound = &Error{Name: "CookieSessionNotFound", Code: 1122, Message: "cookie session not found"}
	CSRFTokenNotFound     = &Error{Name: "CSRFTokenNotFound", Code: 1123, Message: "CSRF token not found"}
	InvalidCSRFToken      = &Error{Name: "InvalidCSRFToken", Code: 1124, Message: "invalid CSRF token"}

	// Auth

	InvalidFlowTicket     = &Error{Name: "InvalidFlowTicket", Code: 1300, Message: "invalid auth flow ticket"}
	FlowTicketIssueFailed = &Error{Name: "FlowTicketIssueFailed", Code: 1301, Message: "failed to issue auth flow ticket"}
	AuthCodeNotFound      = &Error{Name: "AuthCodeNotFound", Code: 1302, Message: "authorization code not found"} // the flow's return request carries no code
	UserNotFound          = &Error{Name: "UserNotFound", Code: 1310, Message: "user not found"}
	UserDisabled          = &Error{Name: "UserDisabled", Code: 1311, Message: "user disabled"}

	// ---- Signed assertions (machine callers)

	AssertionNotFound      = &Error{Name: "AssertionNotFound", Code: 1320, Message: "assertion not found"}           // no Authorization header, or not the assertion scheme
	InvalidAssertion       = &Error{Name: "InvalidAssertion", Code: 1321, Message: "invalid assertion"}              // malformed, bad signature, claim or request-binding mismatch — detail says which
	AssertionReplayed      = &Error{Name: "AssertionReplayed", Code: 1322, Message: "assertion replayed"}            // jti already seen inside its validity window
	AssertionClientUnknown = &Error{Name: "AssertionClientUnknown", Code: 1323, Message: "assertion client unknown"} // iss names no configured client

	// ---- External identity verification (id_token from an IdP or auth server)

	IDTokenInvalid         = &Error{Name: "IDTokenInvalid", Code: 1330, Message: "invalid id_token"}                           // signature, aud/iss/exp, malformed, unknown kid, nonce, rejected claim, no sub — detail says which
	AuthCodeExchangeFailed = &Error{Name: "AuthCodeExchangeFailed", Code: 1331, Message: "authorization code exchange failed"} // the IdP refused the code (stale/replayed code, redirect_uri, PKCE)
	IDPUnavailable         = &Error{Name: "IDPUnavailable", Code: 1332, Message: "identity provider unavailable"}              // JWKS / token endpoint / auth server unreachable or answering garbage
	AuthClientMismatch     = &Error{Name: "AuthClientMismatch", Code: 1333, Message: "auth client mismatch"}                   // a verify request's auth_client_id is not the IdP client configured for the caller

	// Data Format & Serialization

	JSONMarshalFailed   = &Error{Name: "JSONMarshalFailed", Code: 1400, Message: "failed to marshal JSON"}
	JSONUnmarshalFailed = &Error{Name: "JSONUnmarshalFailed", Code: 1401, Message: "failed to unmarshal JSON"}

	// Access Control (Permissions, Resources, Throttling)

	DataMissingInContext = &Error{Name: "DataMissingInContext", Code: 1500, Message: "data missing in context"} // expected ctx attachment absent (middleware misconfiguration / bypass)
	InvalidAuthUID       = &Error{Name: "InvalidAuthUID", Code: 1501, Message: "authenticated user ID missing from context"}
	PermissionDenied     = &Error{Name: "PermissionDenied", Code: 1510, Message: "permission denied"}          // user lacks required permission
	ResourceNotFound     = &Error{Name: "ResourceNotFound", Code: 1520, Message: "resource not found"}         // expected resource must exist but is missing
	ResourceAccessDenied = &Error{Name: "ResourceAccessDenied", Code: 1521, Message: "resource access denied"} // resource exists but user cannot access it
	ResourceUnavailable  = &Error{Name: "ResourceUnavailable", Code: 1522, Message: "resource unavailable"}    // resource exists but is not currently available (temporarily or permanently)
	RateLimited          = &Error{Name: "RateLimited", Code: 1530, Message: "rate limited"}                    // request throttled (per-user / per-session / per-IP bucket exceeded)

	// DB

	KVDB               = &Error{Name: "KVDB", Code: 1600, Message: "kvdb error"}                                     // general key-value store error
	SQLDB              = &Error{Name: "SQLDB", Code: 1610, Message: "sql db error"}                                  // general SQL/database error
	SQLNotFoundInStore = &Error{Name: "SQLNotFoundInStore", Code: 1611, Message: "sql statement not found in store"} // SQL statement not found in RawSQLStore

	// Relation

	RelBelongsToLinkFailed = &Error{Name: "RelBelongsToLinkFailed", Code: 1700, Message: "relation BelongsTo link failed"} // parent not found for child's FK during LinkBelongsTo

	// Upstream

	UpstreamAccessTokenNotFound     = &Error{Name: "UpstreamAccessTokenNotFound", Code: 1800, Message: "upstream access token not found"}   // access token missing to authenticate with an upstream server
	UpstreamRefreshTokenNotFound    = &Error{Name: "UpstreamRefreshTokenNotFound", Code: 1801, Message: "upstream refresh token not found"} // refresh token missing to refresh an upstream access token
	InvalidUpstreamAccessToken      = &Error{Name: "InvalidUpstreamAccessToken", Code: 1802, Message: "invalid upstream access token"}
	InvalidUpstreamRefreshToken     = &Error{Name: "InvalidUpstreamRefreshToken", Code: 1803, Message: "invalid upstream refresh token"}
	Upstream                        = &Error{Name: "Upstream", Code: 1810, Message: "upstream error"}                                                    // failure during upstream interaction (build/transport/server)
	UpstreamUnavailable             = &Error{Name: "UpstreamUnavailable", Code: 1811, Message: "upstream unavailable"}                                   // upstream unreachable, or its gateway answered 502/504 for it — the caller translates for its own users
	UpstreamRefreshSideloaderNotSet = &Error{Name: "UpstreamRefreshSideloaderNotSet", Code: 1820, Message: "upstream refresh sideloader not configured"} // no refresh sideloader registered on Client for this session-data type
	UpstreamTokenCipherNotSet       = &Error{Name: "UpstreamTokenCipherNotSet", Code: 1830, Message: "upstream token cipher not configured"}             // at-rest cipher missing while storing/fetching upstream tokens — KEK boot step skipped

	// Misc

	PDFBuildFailed = &Error{Name: "PDFBuildFailed", Code: 1900, Message: "failed to build PDF"}
)
