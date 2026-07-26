package framework

import "github.com/x64c/gwf/gw/tg"

func (c *Core) PrepareTypedGroupRegistry() {
	c.TypedGroupRegistry = make(map[string]tg.RegGrp)
}
