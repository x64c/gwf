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

	// ---- Cookie Session

	CookieNotFound        = &Error{Name: "CookieNotFound", Code: 1120, Message: "cookie not found"}
	InvalidCookie         = &Error{Name: "InvalidCookie", Code: 1121, Message: "invalid cookie"}
	CookieSessionNotFound = &Error{Name: "CookieSessionNotFound", Code: 1122, Message: "cookie session not found"}
	CSRFTokenNotFound     = &Error{Name: "CSRFTokenNotFound", Code: 1123, Message: "CSRF token not found"}
	InvalidCSRFToken      = &Error{Name: "InvalidCSRFToken", Code: 1124, Message: "invalid CSRF token"}

	// Auth

	InvalidFlowTicket = &Error{Name: "InvalidFlowTicket", Code: 1300, Message: "invalid auth flow ticket"}
	UserNotFound      = &Error{Name: "UserNotFound", Code: 1310, Message: "user not found"}
	UserDisabled      = &Error{Name: "UserDisabled", Code: 1311, Message: "user disabled"}

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
	UpstreamRefreshSideloaderNotSet = &Error{Name: "UpstreamRefreshSideloaderNotSet", Code: 1820, Message: "upstream refresh sideloader not configured"} // no refresh sideloader registered on Client for this session-data type
	UpstreamTokenCipherNotSet       = &Error{Name: "UpstreamTokenCipherNotSet", Code: 1830, Message: "upstream token cipher not configured"}             // at-rest cipher missing while storing/fetching upstream tokens — KEK boot step skipped

	// Misc

	PDFBuildFailed = &Error{Name: "PDFBuildFailed", Code: 1900, Message: "failed to build PDF"}
)
