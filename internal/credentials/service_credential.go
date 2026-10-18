package credentials

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"regexp"
)

const serviceCredentialPrefix = "smsc"

var serviceCredentialRe = regexp.MustCompile(`^smsc_([a-z0-9]+)_([A-Za-z0-9_-]+)$`)

// GenerateServiceCredential returns a new Service credential's visible prefix, its
// secret and the value a service presents (`smsc_<prefix>_<secret>`). The prefix of an
// API token is `omtk`, so one kind never parses as the other.
func GenerateServiceCredential() (prefix, secret, value string, err error) {
	if prefix, err = GenerateTokenPrefix(); err != nil {
		return "", "", "", err
	}
	if secret, err = GenerateTokenSecret(); err != nil {
		return "", "", "", err
	}
	return prefix, secret, ServiceCredentialValue(prefix, secret), nil
}

// ServiceCredentialValue is the value a service presents for a credential.
func ServiceCredentialValue(prefix, secret string) string {
	return serviceCredentialPrefix + "_" + prefix + "_" + secret
}

// ParseServiceCredential splits a presented value into its prefix and secret; nil when
// the value is not shaped like a Service credential.
func ParseServiceCredential(value string) *ParsedToken {
	m := serviceCredentialRe.FindStringSubmatch(value)
	if m == nil {
		return nil
	}
	return &ParsedToken{Prefix: m[1], Secret: m[2]}
}

// HashServiceSecret is the stored form of a Service credential's secret: its SHA-256
// hex. The secret is 40 random characters, so a fast unsalted hash is enough, and it is
// checked on every request, where a slow password hash would be a cost with no gain.
func HashServiceSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// VerifyServiceSecret reports whether secret hashes to the stored hash, in constant time.
func VerifyServiceSecret(secret, hash string) bool {
	return subtle.ConstantTimeCompare([]byte(HashServiceSecret(secret)), []byte(hash)) == 1
}
