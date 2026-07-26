package framework

import (
	"net/http"

	"github.com/x64c/gwf/gw/web"
)

// PrepareWebService
// Call this when all the required services are prepared
func (c *Core) PrepareWebService(addr string, httpHandler http.Handler) {
	c.WebService = web.NewService(addr, httpHandler)
	c.AddService(c.WebService)
}
