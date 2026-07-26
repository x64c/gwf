package handlerwrappers

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/x64c/gwf/gw/web/responses"
)

func RecoverPanic(inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC] recovered: %v\n%s", rec, debug.Stack())
				responses.WriteSimpleErrorJSON(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		inner.ServeHTTP(w, r)
	})
}
