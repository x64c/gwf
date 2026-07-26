package framework

import "html/template"

func (c *Core) PrepareHTMLTemplateStore() {
	c.HTMLTemplateStore = make(map[string]map[string]*template.Template)
}
