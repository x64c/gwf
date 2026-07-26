package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
)

type HtmltplGetAll struct {
	AppProvider framework.AppProviderFunc
}

func (h *HtmltplGetAll) GroupName() string {
	return "htmltpl"
}

func (h *HtmltplGetAll) Command() string {
	return "htmltpl-get-all"
}

func (h *HtmltplGetAll) Desc() string {
	return "Print All Templates in the HTML Template Store"
}

func (h *HtmltplGetAll) Usage() string {
	return h.Command()
}

func (h *HtmltplGetAll) HandleCommand(_ []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()

	store := appCore.HTMLTemplateStore
	if store == nil {
		return fmt.Errorf("html template store not ready")
	}
	for storeKey, templates := range store {
		_, _ = fmt.Fprintf(w, "\n======== Store: %s ========\n", storeKey)
		for key, t := range templates {
			_, _ = fmt.Fprintf(w, "\n________ Template Set: %s ________\n", key)
			// Print entry point first
			_, _ = fmt.Fprintf(w, "\n\t\t[ %s ] ★ entry\n\n", t.Name())
			if t.Tree != nil && t.Tree.Root != nil {
				_, _ = fmt.Fprintln(w, t.Tree.Root.String())
			} else {
				_, _ = fmt.Fprintln(w, "(no AST)")
			}
			_, _ = fmt.Fprintln(w, " ")
			// Print the rest
			for _, tmpl := range t.Templates() {
				if tmpl.Name() == t.Name() {
					continue
				}
				_, _ = fmt.Fprintf(w, "\n\t\t[ %s ]\n\n", tmpl.Name())
				if tmpl.Tree != nil && tmpl.Tree.Root != nil {
					_, _ = fmt.Fprintln(w, tmpl.Tree.Root.String())
				} else {
					_, _ = fmt.Fprintln(w, "(no AST)")
				}
				_, _ = fmt.Fprintln(w, " ")
			}
		}
	}
	_, _ = fmt.Fprintln(w, "________________________________________________")
	return nil
}
