package bearer

// SessionRow is the typed view of a bearer session's persisted KVDB hash row
// (the "umbrella" row keyed by session ID — distinct from the access/refresh
// token rows which are pointers to this one).
//
// Bearer uses a single row shape regardless of identity binds — empty strings
// represent absence (UID="" for userless flows, ClientID="" for clientless
// flows). GroupName ties the row back to its SessionGroupConf for TTL and
// cap-policy lookup at runtime.
type SessionRow struct {
	UID                 string
	ClientID            string
	GroupName           string
	AccessTokenHash     string
	RefreshTokenHash    string
	RefreshChainStartAt int64 // epoch seconds — anchor for refresh_chain_ttl (hardcap)
}
