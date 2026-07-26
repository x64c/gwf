package svc

import "fmt"

type State int

const (
	StateREADY       State = 1
	StateRUNNING     State = 2
	StateSTOPPING    State = 3 // Stop in progress; → READY when stopped channel fires
	StateTERMINATING State = 4 // Terminate in progress; state stays here, Terminated() channel signals truly done
)

func (s State) String() string {
	switch s {
	case StateREADY:
		return "READY"
	case StateRUNNING:
		return "RUNNING"
	case StateSTOPPING:
		return "STOPPING"
	case StateTERMINATING:
		return "TERMINATING"
	default:
		return fmt.Sprintf("State(%d)", int(s))
	}
}

// StateReporter is the read-only view of a service's lifecycle state. A
// sub-component (e.g. a session protocol manager) holds this to consult its
// parent service's state without taking the full Service interface — which
// would also expose Start/Stop/Terminate, lifecycle control a child must not
// have over its parent.
type StateReporter interface {
	State() State
}
