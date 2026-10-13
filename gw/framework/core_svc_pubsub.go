package framework

import (
	"errors"

	"github.com/x64c/gwf/gw/pubsubs"
)

// PreparePubSubService registers broker — built and chosen by the app — as Core's pub/sub service. At runtime
// consumers reach it through PubSubHandle, whose Get refuses while it is stopped.
//
// Prepare it BEFORE PrepareWebService: the web service declares that its handlers may use this service only if it is
// already registered, and that declaration is what has the web service drain before this service stops.
func (c *Core) PreparePubSubService(broker pubsubs.Broker) error {
	if c.webNode != nil {
		return errors.New("PreparePubSubService: the web service is already prepared — prepare the pub/sub service first, so the web service is ordered before it at shutdown")
	}
	if broker == nil {
		return errors.New("PreparePubSubService: no broker")
	}
	// No dependency to declare: what a broker uses beyond Core is its own.
	node, err := c.RegisterService(broker)
	if err != nil {
		return err
	}
	c.pubsubBroker = broker
	c.pubsubNode = node
	return nil
}
