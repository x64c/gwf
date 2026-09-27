package uds

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// promptMarker leads every question a running command asks, as "> " leads a
// command line, so a session waiting for an answer is told apart from one
// waiting for a command.
const promptMarker = ">> "

// Prompter is how a running command asks the operator for more. Answers are
// read through the session's own scanner — the one the command line came
// from — so input sent ahead is never lost, and an answer is capped at
// max_line_bytes like a command line. Answers are never logged. Input is
// visible as it is typed.
type Prompter struct {
	scanner *bufio.Scanner
	w       io.Writer
}

// NewPrompter makes the Prompter of a session that reads lines from scanner
// and writes to w. The service makes one per connection; a test or another
// driver of HandleCommand makes its own.
func NewPrompter(scanner *bufio.Scanner, w io.Writer) *Prompter {
	return &Prompter{scanner: scanner, w: w}
}

// Ask writes ">> " + question and returns the operator's next line as typed —
// not trimmed, its line ending dropped — in a copy the caller owns, so a
// secret can be cleared after use. io.ErrUnexpectedEOF means the session
// ended before an answer; an answer over max_line_bytes is an error wrapping
// bufio.ErrTooLong. A command should not ask while it holds a lock or a
// transaction: the operator may take any time to answer.
func (p *Prompter) Ask(question string) ([]byte, error) {
	if _, err := fmt.Fprint(p.w, promptMarker+question); err != nil {
		return nil, err
	}
	if !p.scanner.Scan() {
		switch err := p.scanner.Err(); {
		case err == nil:
			return nil, io.ErrUnexpectedEOF
		case errors.Is(err, bufio.ErrTooLong):
			return nil, fmt.Errorf("answer exceeds max_line_bytes: %w", err)
		default:
			return nil, err
		}
	}
	return bytes.Clone(p.scanner.Bytes()), nil
}
