// Package credentials generates, hashes and verifies the secrets the application issues or accepts:
// passwords, API tokens, service credentials and recovery codes.
package credentials

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"

	"github.com/getsphericon/sphericon/internal/kernel/randtext"
)

const (
	tokenPrefix      = "omtk"
	collectKeyPrefix = "omck"
	ingestKeyPrefix  = "omik"

	prefixAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	prefixLen      = 12
	secretLen      = 40
)

var tokenRe = prefixedTokenRe(tokenPrefix)

// prefixedTokenRe matches `<kind>_<prefix>_<secret>` and captures the prefix and the secret.
// Both have exactly the generated length, so a value of any other shape is refused before
// it costs a database lookup.
func prefixedTokenRe(kind string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`^%s_([a-z0-9]{%d})_([A-Za-z0-9_-]{%d})$`,
		regexp.QuoteMeta(kind), prefixLen, secretLen))
}

type ParsedToken struct {
	Prefix string
	Secret string
}

// GenerateTokenPrefix returns prefixLen random characters of [a-z0-9]: the public half of an API
// token or a service credential, by which it is looked up.
func GenerateTokenPrefix() string {
	return randtext.FromAlphabet(prefixAlphabet, prefixLen)
}

// GenerateTokenSecret returns secretLen URL-safe base64 characters. secretLen is a multiple of
// 4, so the random bytes fill every character and the last one carries no zero padding bits.
// Like GenerateTokenPrefix it returns no error: crypto/rand.Read never fails (the runtime aborts
// if the system source does).
func GenerateTokenSecret() string {
	b := make([]byte, base64.RawURLEncoding.DecodedLen(secretLen))
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func TokenValue(prefix, secret string) string {
	return tokenPrefix + "_" + prefix + "_" + secret
}

// GenerateCollectKey returns a per-workspace collect write-key (x-collect-key).
func GenerateCollectKey() string {
	return prefixedKey(collectKeyPrefix)
}

// GenerateIngestKey returns an Integration's hook key (POST /hooks/{key}): the
// address of one Integration's inbound provider callbacks, which also names its
// Workspace. Unlike the collect key it is never exposed in the browser tracker.
func GenerateIngestKey() string {
	return prefixedKey(ingestKeyPrefix)
}

// prefixedKey returns `<kind>_<secret>` with a fresh secret.
func prefixedKey(kind string) string {
	return kind + "_" + GenerateTokenSecret()
}

// HashSecret is the stored form of a generated secret: its SHA-256 hex.
// The secret is 240 random bits, so a fast unsalted hash is enough, and it is checked on every request,
// where a slow password hash would be a cost with no gain.
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// VerifySecret reports whether secret hashes to the stored hash, in constant time.
func VerifySecret(secret, hash string) bool {
	return subtle.ConstantTimeCompare([]byte(HashSecret(secret)), []byte(hash)) == 1
}

func ParseToken(value string) *ParsedToken {
	return parseWith(tokenRe, value)
}

// parseWith splits value by a prefixedTokenRe pattern; nil when it does not match.
func parseWith(re *regexp.Regexp, value string) *ParsedToken {
	m := re.FindStringSubmatch(value)
	if m == nil {
		return nil
	}
	return &ParsedToken{Prefix: m[1], Secret: m[2]}
}
