package cookie

// UserSessionRow is the typed view of a user session's persisted KVDB hash row.
type UserSessionRow struct {
	UID  string
	CSRF string
}

// AnonymousSessionRow is the typed view of an anonymous session's persisted KVDB hash row.
type AnonymousSessionRow struct {
	CSRF string
}
