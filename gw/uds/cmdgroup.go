package uds

type CommandGroup struct {
	name         string
	handlerMap   map[string]CommandHandler
	displayOrder []string
}
