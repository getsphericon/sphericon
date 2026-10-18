// Package licensekey is the runtime EE license gate (ADR 0014, mechanism a): one binary
// ships to everyone, and an Enterprise feature runs only when a valid offline license
// key is present. It is unrelated to the hosted offering (ADR 0014).
//
// A key is a JWT signed with Ed25519 (EdDSA): the licensed features are the `features`
// claim and the optional expiry is the standard `exp`. Verification needs only the
// public key, so nothing secret ships.
//
// Governed by ee/LICENSE, not the AGPL.
package licensekey

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Feature names one licensed Enterprise capability.
type Feature string

const (
	// FeatureAudit unlocks the Audit log (ADR 0022).
	FeatureAudit Feature = "audit"
	// FeatureRetention unlocks the advanced-retention control (ADR 0014): a per-Workspace
	// window after which Enterprise records, today the Audit log, are pruned.
	FeatureRetention Feature = "retention"
	// FeatureStaff unlocks the Staff member, its /staff surface and console, and the
	// abuse detector (ADR 0026): one feature, meaningful only together.
	FeatureStaff Feature = "staff"
)

// ProductionKey is the Ed25519 public key production licenses are verified against.
// The matching private key stays with the license issuer.
var ProductionKey = mustDecodeKey("GAQfTzJEjCIbmnwiSJLtjL/aiEwabOdZ2SVYIG9ZxZk=")

// ErrInvalid means a non-empty key failed verification: malformed, forged or expired.
var ErrInvalid = errors.New("licensekey: invalid key")

// License is the set of licensed features. The zero value and nil license nothing.
type License struct {
	features map[Feature]bool
}

// Has reports whether the feature is licensed. Safe on a nil License.
func (l *License) Has(f Feature) bool {
	return l != nil && l.features[f]
}

type claims struct {
	Features []Feature `json:"features"`
	jwt.RegisteredClaims
}

// Parse verifies key against pub at time now. An empty key is not an error: it is the
// unlicensed core, and yields a License that licenses nothing. Anything else that does
// not verify returns ErrInvalid, so a mistyped key fails at boot instead of silently
// running unlicensed.
func Parse(key string, pub ed25519.PublicKey, now time.Time) (*License, error) {
	if key == "" {
		return &License{}, nil
	}
	var c claims
	if _, err := jwt.ParseWithClaims(key, &c, func(*jwt.Token) (any, error) { return pub, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithTimeFunc(func() time.Time { return now }),
	); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	l := &License{features: make(map[Feature]bool, len(c.Features))}
	for _, f := range c.Features {
		l.features[f] = true
	}
	return l, nil
}

// Issue signs a key for the features with priv. A nil expires never lapses. The
// production private key is not in this repository; tests mint their own key pair.
func Issue(priv ed25519.PrivateKey, expires *time.Time, features ...Feature) (string, error) {
	c := claims{Features: features}
	if expires != nil {
		c.ExpiresAt = jwt.NewNumericDate(*expires)
	}
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, c).SignedString(priv)
}

func mustDecodeKey(s string) ed25519.PublicKey {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic("licensekey: bad embedded public key")
	}
	return b
}
