package licensekey

import (
	"crypto/ed25519"
	"crypto/sha256"
)

// devSeed derives the development key pair. It is public on purpose: a key it signs is trusted
// only by a source build outside production (VerifyingKey, ADR 0060), so it unlocks nothing in a
// release.
const devSeed = "sphericon development license key"

// DevPrivateKey is the private half of the development key pair, for minting a key that a
// development or end-to-end Instance accepts.
func DevPrivateKey() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte(devSeed))
	return ed25519.NewKeyFromSeed(seed[:])
}

// VerifyingKey is the public key an Instance verifies its license key against: the production
// key, or with trustDev the development one, so the dev stack and the end-to-end suite can run a
// licensed Instance. The caller says when that is safe (config.Config.TrustsDevLicense).
func VerifyingKey(trustDev bool) ed25519.PublicKey {
	if trustDev {
		return ed25519.PublicKey(DevPrivateKey()[ed25519.SeedSize:])
	}
	return ProductionKey
}

// IssueDev signs a key with every feature, for DevPrivateKey's pair, that never expires.
func IssueDev() (string, error) {
	return Issue(DevPrivateKey(), nil, FeatureAudit, FeatureRetention, FeatureStaff, FeatureExport)
}
