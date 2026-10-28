package app

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/licensekey"
)

func TestRequireStaffLicenseForSaaS(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	parse := func(features ...licensekey.Feature) *licensekey.License {
		key := ""
		if len(features) > 0 {
			key, err = licensekey.Issue(priv, nil, features...)
			require.NoError(t, err)
		}
		lic, err := licensekey.Parse(key, pub, time.Now())
		require.NoError(t, err)
		return lic
	}

	require.ErrorContains(t, requireStaffLicenseForSaaS(config.ModeSaaS, parse()), "staff", "saas without a key must not start")
	require.ErrorContains(t, requireStaffLicenseForSaaS(config.ModeSaaS, parse(licensekey.FeatureAudit)), "staff", "a key without staff is not enough")
	require.NoError(t, requireStaffLicenseForSaaS(config.ModeSaaS, parse(licensekey.FeatureStaff)))
	require.NoError(t, requireStaffLicenseForSaaS(config.ModeSelfHosted, parse()), "selfhosted needs no key")
}
