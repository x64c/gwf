package uds

import "io"

type CommandHandler interface {
	Command() string // Unique Name
	GroupName() string
	Desc() string
	Usage() string
	HandleCommand(args []string, w io.Writer) error
}
