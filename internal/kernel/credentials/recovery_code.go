package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HashRecoveryCode is the stored form of a Recovery code (ADR 0020): the SHA-256 hex
// of the code lower-cased with spaces and dashes removed, so it may be typed either
// way. The codes are random, so an unsalted fast hash is enough, and the lookup by
// hash must be deterministic.
func HashRecoveryCode(code string) string {
	norm := strings.NewReplacer("-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(code)))
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}
