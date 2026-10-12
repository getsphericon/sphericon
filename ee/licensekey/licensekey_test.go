package licensekey_test

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
)

func TestParse(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, otherPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)

	valid, err := licensekey.Issue(priv, &future, licensekey.FeatureAudit)
	require.NoError(t, err)
	forever, err := licensekey.Issue(priv, nil, licensekey.FeatureAudit)
	require.NoError(t, err)
	expired, err := licensekey.Issue(priv, &past, licensekey.FeatureAudit)
	require.NoError(t, err)
	forged, err := licensekey.Issue(otherPriv, nil, licensekey.FeatureAudit)
	require.NoError(t, err)
	noFeatures, err := licensekey.Issue(priv, nil)
	require.NoError(t, err)

	l, err := licensekey.Parse(valid, pub, now)
	require.NoError(t, err)
	assert.True(t, l.Has(licensekey.FeatureAudit))

	l, err = licensekey.Parse(forever, pub, now)
	require.NoError(t, err)
	assert.True(t, l.Has(licensekey.FeatureAudit), "no expiry never lapses")

	l, err = licensekey.Parse(noFeatures, pub, now)
	require.NoError(t, err)
	assert.False(t, l.Has(licensekey.FeatureAudit), "a key licenses only the features it names")

	l, err = licensekey.Parse("", pub, now)
	require.NoError(t, err, "no key is the unlicensed core, not an error")
	assert.False(t, l.Has(licensekey.FeatureAudit))

	for name, key := range map[string]string{"expired": expired, "forged": forged, "garbage": "nonsense", "bad base64": "!!.!!"} {
		_, err := licensekey.Parse(key, pub, now)
		assert.ErrorIs(t, err, licensekey.ErrInvalid, name)
	}

	var none *licensekey.License
	assert.False(t, none.Has(licensekey.FeatureAudit), "nil license licenses nothing")
}
