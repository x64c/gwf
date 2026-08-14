package uds

import (
	"fmt"
	"io"
)

type CommandStore struct {
	handlerMap        map[string]CommandHandler
	groupMap          map[string]*CommandGroup
	groupDisplayOrder []string
}

func NewCommandStore(cmdHandlers ...CommandHandler) (*CommandStore, error) {
	s := &CommandStore{
		handlerMap:        make(map[string]CommandHandler),
		groupMap:          make(map[string]*CommandGroup),
		groupDisplayOrder: make([]string, 0),
	}
	if err := s.SetCommandHandlers(cmdHandlers...); err != nil {
		return nil, err
	}
	return s, nil
}

// NewCommandStoreOrPanic is NewCommandStore for a package-level var, where an
// error has nowhere to go.
// WARNING: This function panics if a command is declared twice or in
// conflicting groups. Package initialization runs before main, so the failure
// lands as a failed program load — nothing has bound a listener or opened a
// pool yet. See sqldbs.NewTableOrPanic for the same rationale.
func NewCommandStoreOrPanic(cmdHandlers ...CommandHandler) *CommandStore {
	s, err := NewCommandStore(cmdHandlers...)
	if err != nil {
		panic(err)
	}
	return s
}

// SetCommandHandler registers one handler, and reports a command claimed by
// two different groups.
func (s *CommandStore) SetCommandHandler(cmdHandler CommandHandler) error {
	cmd := cmdHandler.Command()
	grpName := cmdHandler.GroupName()

	prevHnd, hndExists := s.handlerMap[cmd]
	grp, grpExists := s.groupMap[grpName]

	if hndExists {
		// found the previous cmd -> group must exists and match
		if grpName != prevHnd.GroupName() {
			return fmt.Errorf("uds: conflict command %q to set in groups: %q vs %q", cmd, prevHnd.GroupName(), grpName)
		}
		if !grpExists {
			return fmt.Errorf("uds: missing group %q", grpName)
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
	return nil
}

func (s *CommandStore) SetCommandHandlers(cmdHandlers ...CommandHandler) error {
	for _, cmdHandler := range cmdHandlers {
		if err := s.SetCommandHandler(cmdHandler); err != nil {
			return err
		}
	}
	return nil
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
