package licensekey_test

import (
	"crypto/ed25519"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
	expiredForged, err := licensekey.Issue(otherPriv, &past, licensekey.FeatureAudit)
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

	l, err = licensekey.Parse(expired, pub, now)
	require.NoError(t, err, "an expired key loads as the core")
	assert.False(t, l.Has(licensekey.FeatureAudit))
	at, lapsed := l.LapsedAt()
	assert.True(t, lapsed)
	assert.True(t, at.Equal(past.Truncate(time.Second)))
	_, lapsed = valid2(t, valid, pub, now).LapsedAt()
	assert.False(t, lapsed)

	_, err = licensekey.Parse(expiredForged, pub, now)
	require.ErrorIs(t, err, licensekey.ErrInvalid, "a forged key is refused even when expired")

	for name, key := range map[string]string{"forged": forged, "garbage": "nonsense", "bad base64": "!!.!!.!!"} {
		_, err := licensekey.Parse(key, pub, now)
		require.ErrorIs(t, err, licensekey.ErrInvalid, name)
	}

	var none *licensekey.License
	assert.False(t, none.Has(licensekey.FeatureAudit), "nil license licenses nothing")
}

func valid2(t *testing.T, key string, pub ed25519.PublicKey, now time.Time) *licensekey.License {
	t.Helper()
	l, err := licensekey.Parse(key, pub, now)
	require.NoError(t, err)
	return l
}

func TestLoadWarnsWithTheExpiryDateOfALapsedKey(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	past := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	key, err := licensekey.Issue(priv, &past, licensekey.FeatureAudit)
	require.NoError(t, err)

	var out strings.Builder
	l, err := licensekey.Load(key, pub, now, slog.New(slog.NewTextHandler(&out, nil)))
	require.NoError(t, err)
	assert.False(t, l.Has(licensekey.FeatureAudit))
	assert.Contains(t, out.String(), "level=WARN")
	assert.Contains(t, out.String(), "expired=2026-05-20")

	out.Reset()
	_, err = licensekey.Load("", pub, now, slog.New(slog.NewTextHandler(&out, nil)))
	require.NoError(t, err)
	assert.Empty(t, out.String(), "no key is the core, not a lapse")
}

func TestParseRefusesAKeyThatIsNotValidYet(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	key, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
		"features": []string{"audit"}, "nbf": now.Add(time.Hour).Unix(),
	}).SignedString(priv)
	require.NoError(t, err)

	_, err = licensekey.Parse(key, pub, now)

	assert.ErrorIs(t, err, licensekey.ErrInvalid)
}

func TestDevKeyIsTrustedOnlyWhenTheCallerSaysSo(t *testing.T) {
	key, err := licensekey.IssueDev()
	require.NoError(t, err)

	l, err := licensekey.Parse(key, licensekey.VerifyingKey(true), time.Now())
	require.NoError(t, err)
	assert.True(t, l.Has(licensekey.FeatureStaff), "a development build accepts the dev key")

	_, err = licensekey.Parse(key, licensekey.VerifyingKey(false), time.Now())
	assert.ErrorIs(t, err, licensekey.ErrInvalid, "a release must refuse the dev key")
}
