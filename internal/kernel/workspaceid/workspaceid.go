// Package workspaceid makes the Workspace id (ADR 0043): 8 random base62 characters, the one
// address a Workspace has. It is a leaf package so that Workspace creation and the migration
// that backfills existing Workspaces share one generator.
package workspaceid

import (
	"crypto/rand"
	"regexp"
)

const (
	// Length is the number of characters in a Workspace id.
	Length = 8

	alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	// unbiasedBelow is the largest multiple of the alphabet size that fits in a byte: a random
	// byte at or above it is discarded so every character is equally likely.
	unbiasedBelow = 256 - 256%len(alphabet)
)

// Pattern matches a well-formed Workspace id.
var Pattern = regexp.MustCompile(`^[0-9A-Za-z]{8}$`)

// New returns a fresh random Workspace id. Uniqueness is the database's to enforce: a caller
// that hits a unique violation asks for another.
func New() string {
	id := make([]byte, 0, Length)
	buf := make([]byte, Length*2)
	for len(id) < Length {
		// crypto/rand.Read never returns an error: the runtime aborts if the system source fails.
		_, _ = rand.Read(buf)
		for _, b := range buf {
			if int(b) >= unbiasedBelow {
				continue
			}
			id = append(id, alphabet[int(b)%len(alphabet)])
			if len(id) == Length {
				break
			}
		}
	}
	return string(id)
}
