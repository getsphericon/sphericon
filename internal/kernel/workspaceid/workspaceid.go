// Package workspaceid makes the Workspace id (ADR 0043): 8 random base62 characters, the one
// address a Workspace has. It is a kernel package, importing only the kernel, so that Workspace creation and the migration
// that backfills existing Workspaces share one generator.
package workspaceid

import (
	"regexp"

	"github.com/getsphericon/sphericon/internal/kernel/randtext"
)

const (
	// Length is the number of characters in a Workspace id.
	Length = 8

	alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// Pattern matches a well-formed Workspace id.
var Pattern = regexp.MustCompile(`^[0-9A-Za-z]{8}$`)

// New returns a fresh random Workspace id. Uniqueness is the database's to enforce: a caller
// that hits a unique violation asks for another.
func New() string {
	return randtext.FromAlphabet(alphabet, Length)
}
