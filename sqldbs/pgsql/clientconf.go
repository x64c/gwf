package pgsql

// ClientConf holds shared PostgreSQL server connection config.
type ClientConf struct {
	Host string    `json:"host"`
	Port int       `json:"port"`
	User string    `json:"user"`
	PW   string    `json:"pw"`
	TZ   string    `json:"tz"`
	DSN  string    `json:"dsn"`  // To overwrite default DSN building — must not carry pool_* (the pool block is the single source)
	Pool *PoolConf `json:"pool"` // REQUIRED — see PoolConf

	// InitTimeoutSecs is the deadline for EACH database's whole
	// initialization in CreateDB: connect + ping + the schema snapshot.
	// REQUIRED (seconds > 0). A tolerance judgment only the deployment can
	// make — a local socket may want fail-fast, a managed server resuming
	// from cold or a large schema over a slow link may need far more. Unset,
	// the framework used to hardcode 5 seconds.
	InitTimeoutSecs int `json:"init_timeout_secs"`
}
