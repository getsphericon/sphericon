package testhelper

import (
	"crypto/ed25519"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
)

// License mints a License with exactly features (none: the core's), signed by a throwaway key pair
// and verified through the same Parse the production composition root uses. A test of a module
// that reads the License hands it the one its case needs; Setup's Instance carries testLicense.
func License(tb testing.TB, features ...licensekey.Feature) *licensekey.License {
	tb.Helper()
	lic, err := mintLicense(features...)
	require.NoError(tb, err, "mint test license")
	return lic
}

// testLicense is the harness's License, minted once per process: every feature.
var testLicense = sync.OnceValues(func() (*licensekey.License, error) {
	return mintLicense(licensekey.FeatureAudit, licensekey.FeatureRetention, licensekey.FeatureStaff, licensekey.FeatureExport)
})

func mintLicense(features ...licensekey.Feature) (*licensekey.License, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	key, err := licensekey.Issue(priv, nil, features...)
	if err != nil {
		return nil, err
	}
	return licensekey.Parse(key, pub, time.Now())
}
