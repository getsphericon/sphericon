package erasure

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Hash is how Erasure names an address or a phone number to a consumer of the Erasures
// Export stream (ADR 0045): the lowercase hex SHA-256 of the value, and empty for an empty
// value. Erasure hashes before it publishes, so no address or phone reaches the outbox; a
// consumer hashes its own copy of a value the same way to find the rows to delete.
func Hash(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// firstDestination is the Target's address, lowercased; empty when it has none.
func firstDestination(t *Target) string {
	if len(t.Destinations) == 0 {
		return ""
	}
	return strings.ToLower(t.Destinations[0])
}
