package mysql

// ClientConf holds shared MySQL server connection config.
type ClientConf struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	PW   string `json:"pw"`
	TZ   string `json:"tz"`
	DSN  string `json:"dsn"` // To overwrite default DSN building
}
