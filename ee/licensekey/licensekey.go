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
	"log/slog"
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
	// FeatureStaff unlocks the Enterprise sections of the Staff console (the cross-Workspace
	// views and actions, Staff holds, Service credentials) and the abuse detector (ADR 0026,
	// ADR 0063): one feature, meaningful only together. The Staff member, its sign-in and the
	// console itself are core.
	FeatureStaff Feature = "staff"
	// FeatureExport unlocks the Export streams (ADR 0045): the pull API a Workspace reads its
	// rows through into its own warehouse.
	FeatureExport Feature = "export"
)

// ProductionKey is the Ed25519 public key production licenses are verified against.
// The matching private key stays with the license issuer.
var ProductionKey = mustDecodeKey("QUXvHjTiKlcXQBD1EdbPCMViXZbXUnzdmTBlba4CzTA=")

// ErrInvalid means a non-empty key failed verification: malformed or forged. An expired key
// with a valid signature is not an error (ADR 0035): it loads as a License that licenses nothing.
var ErrInvalid = errors.New("licensekey: invalid key")

// License is the set of licensed features. The zero value and nil license nothing.
type License struct {
	features map[Feature]bool
	lapsedAt time.Time
}

// LapsedAt returns when the key expired and true, or false when the key has not lapsed (or
// there is no key). A lapsed License licenses nothing (ADR 0035). Safe on a nil License.
func (l *License) LapsedAt() (time.Time, bool) {
	if l == nil || l.lapsedAt.IsZero() {
		return time.Time{}, false
	}
	return l.lapsedAt, true
}

// Has reports whether the feature is licensed. Safe on a nil License.
func (l *License) Has(f Feature) bool {
	return l != nil && l.features[f]
}

type claims struct {
	jwt.RegisteredClaims

	Features []Feature `json:"features"`
}

// Parse verifies key against pub at time now. An empty key is not an error: it is the
// unlicensed core, and yields a License that licenses nothing. So does a key with a valid
// signature past its expiry (ADR 0035): the License reports when it lapsed (LapsedAt). Anything
// else that does not verify returns ErrInvalid, so a mistyped or forged key fails at boot
// instead of silently running unlicensed.
func Parse(key string, pub ed25519.PublicKey, now time.Time) (*License, error) {
	if key == "" {
		return &License{}, nil
	}
	var c claims
	if _, err := jwt.ParseWithClaims(key, &c, func(*jwt.Token) (any, error) { return pub, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithoutClaimsValidation(), // the signature is verified; expiry is judged below
	); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if c.NotBefore != nil && now.Before(c.NotBefore.Time) {
		return nil, fmt.Errorf("%w: not valid before %s", ErrInvalid, c.NotBefore.Time.UTC().Format(time.RFC3339))
	}
	if c.ExpiresAt != nil && !now.Before(c.ExpiresAt.Time) {
		return &License{lapsedAt: c.ExpiresAt.Time}, nil
	}
	l := &License{features: make(map[Feature]bool, len(c.Features))}
	for _, f := range c.Features {
		l.features[f] = true
	}
	return l, nil
}

// Load is Parse for the composition root: a lapsed key logs a warning naming its expiry
// date, since the Instance now starts as the core instead of failing (ADR 0035).
func Load(key string, pub ed25519.PublicKey, now time.Time, log *slog.Logger) (*License, error) {
	l, err := Parse(key, pub, now)
	if err != nil {
		return nil, err
	}
	if at, ok := l.LapsedAt(); ok {
		log.Warn("the license key expired: running the open-source core until a valid key is set",
			"expired", at.UTC().Format(time.DateOnly))
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
