// Package inuse is the one shape of a refused removal (ADR 0049): a row that live work still
// uses is not removed, and the refusal names the users. Each domain module returns an *Error
// from its delete; each API surface renders it as its contract's InUseProblem (409, `usedBy`).
package inuse

import (
	"errors"
	"fmt"
	"strings"
)

// Kind is what a user of a row is.
type Kind string

// The kinds of live user a removal can find; each has a value of the contract's UsedByKind.
const (
	// Broadcast is a Broadcast that has not finished sending.
	Broadcast Kind = "broadcast"
)

// User is one row that uses the row a removal was refused for.
type User struct {
	Kind Kind
	ID   int64
	Name string
}

// Error is a removal refused because Users still use the row. Callers match it with As.
type Error struct {
	// What is the removed row, for the message: "segment".
	What  string
	Users []User
}

func (e *Error) Error() string {
	names := make([]string, len(e.Users))
	for i, u := range e.Users {
		names[i] = fmt.Sprintf("%s %q", u.Kind, u.Name)
	}
	return fmt.Sprintf("%s is in use by %s", e.What, strings.Join(names, ", "))
}

// As reports the refusal an error carries, if it is one.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
