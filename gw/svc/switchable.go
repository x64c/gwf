package svc

// Switchable is anything that can be independently enabled or disabled.
type Switchable interface {
	Enable()
	Disable()
	Enabled() bool
}
