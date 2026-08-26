package handlerwrappers

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/responses"
)

// RecoverPanic recovers a panic raised by inner, logs it with a stack trace,
// and writes a 500 JSON error. Its shape is a wrap step: apply it directly, or
// call it from a HandlerWrapper's Wrap.
func RecoverPanic(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC] recovered: %v\n%s", rec, debug.Stack())
				responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError)
			}
		}()
		inner.ServeHTTP(w, r)
	})
}
