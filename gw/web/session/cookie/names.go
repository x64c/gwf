package cookie

// Shape-specific cookie names. RFC-6265bis `__Host-` prefix.
const (
	UserCookieName            = "__Host-session"
	UserIntendedURICookieName = "__Host-intended-uri"
	AnonymousCookieName       = "__Host-public-session"
)
