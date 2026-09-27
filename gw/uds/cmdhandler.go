package uds

import "io"

type CommandHandler interface {
	Command() string // Unique Name
	GroupName() string
	Desc() string
	Usage() string
	// HandleCommand runs the command: args after its name, p for asking the
	// operator more (a handler that asks nothing ignores it), w for the answer.
	HandleCommand(args []string, p *Prompter, w io.Writer) error
}
