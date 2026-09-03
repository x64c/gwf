package coord

import "fmt"

// Mode is an app's coordination identity: where the state its instances
// must agree on — locks, caps, replay windows, throttle budgets — lives, and
// so who contends for it. It is read once, from the app's .core.json
// ("coord_mode": "inproc" | "crossproc"), at framework.NewCore, and derived
// from everywhere else: each Prepare* reads it and seats the matching
// implementation, so no seat can disagree with the mode. The zero value is
// not a mode; no app gets one by omission.
//
// Together with the app's name it says what an instance belongs to: the
// processes running under CrossProc with the same app name against the same
// shared store are one distributed app; a different name is a different app,
// whatever the binary. Identity is set by the deployment's conf, not by the
// build.
type Mode int

const (
	_ Mode = iota // zero = undeclared, refused at NewCore

	// InProc: coordination state lives in process memory; the contenders are
	// the goroutines of one process. Launching a second instance of an InProc
	// app splits that state between two processes that cannot see each other
	// — the single-instance contract is the deployment's to honor.
	InProc

	// CrossProc: coordination state lives in the app's shared store, where
	// every process of the app — every process running under this mode with
	// the app's name against that store — contends for it, so instances may
	// run side by side. A feature whose shared state is process memory only runs per
	// instance under CrossProc; each such feature says so in its own doc.
	CrossProc
)

// ParseMode reads a mode as .core.json spells it: "inproc" or "crossproc".
// Anything else, the empty string included, is an error — a mode is a
// choice, never a default.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "inproc":
		return InProc, nil
	case "crossproc":
		return CrossProc, nil
	default:
		return 0, fmt.Errorf("coordination mode %q undeclared — use \"inproc\" or \"crossproc\"", s)
	}
}

func (m Mode) String() string {
	switch m {
	case InProc:
		return "inproc"
	case CrossProc:
		return "crossproc"
	default:
		return "undeclared"
	}
}
