package web

// ServerConf is the HTTP server's own configuration, loaded from
// config/.web-server.json by framework.PrepareWebService.
type ServerConf struct {
	DrainTimeoutSecs int `json:"drain_timeout_secs"` // REQUIRED (> 0, < core terminate_timeout_secs). Graceful-drain window for Server.Shutdown on stop.
}
