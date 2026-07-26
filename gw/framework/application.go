package framework

type Application interface {
	AppCore() *Core
}

type AppProviderFunc func() Application
