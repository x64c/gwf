package passwd

import (
	"net/http"

	"github.com/x64c/gwf/gw/web/handlerwrappers"
)

// NameThrottleKey is a throttle key by the name a login request carries, for
// handlerwrappers.ThrottleFixedGroup on a login route: it bounds guesses at one account however
// many addresses they come from — what a per-IP throttle cannot. The name is normalized by the
// app's own rule (the one its lookup applies), so "Foo" and "foo" are one key; an unknown name
// is keyed like a known one, so the throttle's answers do not reveal which names exist.
//
//   - Returns: the key provider — "name:" + normalize(name), true; false for a request it cannot
//     read, which the throttle answers 429.
//   - Uses: passwd.readLoginRequest (a JSON body is put back for the handler).
func NameThrottleKey(normalize func(string) string) handlerwrappers.ThrottleKeyProvider {
	return func(r *http.Request) (string, bool) {
		name, password, e := readLoginRequest(r)
		clear(password)
		if e != nil {
			return "", false
		}
		return "name:" + normalize(name), true
	}
}
