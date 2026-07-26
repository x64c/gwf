package uds

import (
	"fmt"
	"io"
	"log"
)

type CommandStore struct {
	handlerMap        map[string]CommandHandler
	groupMap          map[string]*CommandGroup
	groupDisplayOrder []string
}

func NewCommandStore(cmdHandlers ...CommandHandler) *CommandStore {
	s := &CommandStore{
		handlerMap:        make(map[string]CommandHandler),
		groupMap:          make(map[string]*CommandGroup),
		groupDisplayOrder: make([]string, 0),
	}
	s.SetCommandHandlers(cmdHandlers...)
	return s
}

// SetCommandHandler
// [Conflict] if the cmd already exists in a Different Group -> log.Fatal
func (s *CommandStore) SetCommandHandler(cmdHandler CommandHandler) {
	cmd := cmdHandler.Command()
	grpName := cmdHandler.GroupName()

	prevHnd, hndExists := s.handlerMap[cmd]
	grp, grpExists := s.groupMap[grpName]

	if hndExists {
		// found the previous cmd -> group must exists and match
		if grpName != prevHnd.GroupName() {
			log.Fatalf("[ERROR][UDS] conflict command %q to set in groups: %q vs %q", cmd, prevHnd.GroupName(), grpName)
		}
		if !grpExists {
			log.Fatalf("[ERROR][UDS] missing group %q", grpName)
		}
	} else {
		// New Command
		if !grpExists {
			// New Group
			grp = &CommandGroup{
				name:         grpName,
				handlerMap:   make(map[string]CommandHandler),
				displayOrder: make([]string, 0),
			}
			s.groupDisplayOrder = append(s.groupDisplayOrder, grpName)
			s.groupMap[grpName] = grp
		}
		grp.displayOrder = append(grp.displayOrder, cmd)
	}

	grp.handlerMap[cmd] = cmdHandler
	s.handlerMap[cmd] = cmdHandler
}

func (s *CommandStore) SetCommandHandlers(cmdHandlers ...CommandHandler) {
	for _, cmdHandler := range cmdHandlers {
		s.SetCommandHandler(cmdHandler)
	}
}

func (s *CommandStore) GetHandler(cmd string) (CommandHandler, bool) {
	handler, ok := s.handlerMap[cmd]
	return handler, ok
}

func (s *CommandStore) PrintHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w)
	for _, grpName := range s.groupDisplayOrder {
		cmdGrp, ok := s.groupMap[grpName]
		if !ok {
			continue
		}
		_, _ = fmt.Fprintf(w, "---- %s ----\n", grpName)
		for _, cmd := range cmdGrp.displayOrder {
			cmdHandler, ok := cmdGrp.handlerMap[cmd]
			if !ok {
				continue
			}
			_, _ = fmt.Fprintf(w, "%-36s %s\n", cmd, cmdHandler.Desc())
		}
		_, _ = fmt.Fprintln(w)
	}
	_, _ = fmt.Fprintln(w)
}
