package web

// SessionIDProvider is implemented by any per-request session-data type
// (cookie.UserSessionData, bearer.UserSessionData, bearer.UserlessSessionData,
// etc.) to expose the session's KVDB-row identifier.
//
// Framework code that needs the session ID without committing to a specific
// session-data type or generic UID parameter should accept this interface.
type SessionIDProvider interface {
	SessionID() string
}
