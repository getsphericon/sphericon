// Package licensekey is the runtime EE license gate (ADR 0014, mechanism a): one binary
// ships to everyone, and an Enterprise feature runs only when a valid offline license
// key is present. It is unrelated to SaaS subscription billing (ADR 0009).
//
// A key is `<payload>.<signature>`, both base64url: the payload is JSON naming the
// licensed features and an optional expiry, the signature is Ed25519 over the payload
// bytes. Verification needs only the public key, so nothing secret ships.
//
// Governed by ee/LICENSE, not the AGPL.
package licensekey

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Feature names one licensed Enterprise capability.
type Feature string

const (
	// FeatureAudit unlocks the Audit log (ADR 0022).
	FeatureAudit Feature = "audit"
	// FeatureRetention unlocks the advanced-retention control (ADR 0014): a per-Workspace
	// window after which Enterprise records, today the Audit log, are pruned.
	FeatureRetention Feature = "retention"
	// FeatureOperator unlocks the Operator, its /operator surface and console, and the
	// abuse detector (ADR 0026): one feature, meaningful only together.
	FeatureOperator Feature = "operator"
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

type payload struct {
	Features []Feature  `json:"features"`
	Expires  *time.Time `json:"expires,omitempty"`
}

// Parse verifies key against pub at time now. An empty key is not an error: it is the
// unlicensed core, and yields a License that licenses nothing. Anything else that does
// not verify returns ErrInvalid, so a mistyped key fails at boot instead of silently
// running unlicensed.
func Parse(key string, pub ed25519.PublicKey, now time.Time) (*License, error) {
	if key == "" {
		return &License{}, nil
	}
	body, sig, ok := strings.Cut(key, ".")
	if !ok {
		return nil, fmt.Errorf("%w: malformed", ErrInvalid)
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed", ErrInvalid)
	}
	signature, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !ed25519.Verify(pub, raw, signature) {
		return nil, fmt.Errorf("%w: bad signature", ErrInvalid)
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%w: malformed", ErrInvalid)
	}
	if p.Expires != nil && !now.Before(*p.Expires) {
		return nil, fmt.Errorf("%w: expired", ErrInvalid)
	}
	l := &License{features: make(map[Feature]bool, len(p.Features))}
	for _, f := range p.Features {
		l.features[f] = true
	}
	return l, nil
}

// Issue signs a key for the features with priv. The production private key is not in
// this repository; tests mint their own key pair.
func Issue(priv ed25519.PrivateKey, expires *time.Time, features ...Feature) (string, error) {
	raw, err := json.Marshal(payload{Features: features, Expires: expires})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw) + "." +
		base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, raw)), nil
}

func mustDecodeKey(s string) ed25519.PublicKey {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic("licensekey: bad embedded public key")
	}
	return b
}
