package framework

import (
	"errors"

	"github.com/x64c/gwf/gw/web/sse"
)

// PrepareSSEService validates conf, creates the SSE service and registers it with Core. The returned pointer is for
// boot wiring; at runtime handlers reach the service through SSEHandle, whose Get refuses while it is stopped.
//
// Prepare it BEFORE PrepareWebService: the web service declares that its handlers may use this service only if it is
// already registered, and that declaration is what has the web service drain before this service stops.
func (c *Core) PrepareSSEService(conf sse.Conf) (*sse.Service, error) {
	if c.webNode != nil {
		return nil, errors.New("PrepareSSEService: the web service is already prepared — prepare the SSE service first, so the web service is ordered before it at shutdown")
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	c.sseService = sse.NewService(conf)
	// No dependency to declare: it uses no other service.
	node, err := c.RegisterService(c.sseService)
	if err != nil {
		return nil, err
	}
	c.sseNode = node
	return c.sseService, nil
}
