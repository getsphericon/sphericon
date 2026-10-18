package ee_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/internal/accounts"
)

func licenseFor(t *testing.T, features ...licensekey.Feature) *licensekey.License {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key, err := licensekey.Issue(priv, nil, features...)
	require.NoError(t, err)
	lic, err := licensekey.Parse(key, pub, time.Now())
	require.NoError(t, err)
	return lic
}

var (
	siteSecret = rand.Text() + rand.Text()
	attempts   = accounts.NewAttempts(nil)
)

func TestTheStaffSecretIsRequiredOnlyWithTheStaffLicense(t *testing.T) {
	unset := staff.Config{SessionTTL: time.Hour, Attempts: attempts}

	e, err := ee.New(nil, licenseFor(t, licensekey.FeatureAudit), nil, siteSecret, unset)
	require.NoError(t, err, "an instance without the staff license needs no staff secret")
	assert.Nil(t, e.Staff(nil, nil), "and has no staff surface")

	_, err = ee.New(nil, licenseFor(t, licensekey.FeatureStaff), nil, siteSecret, unset)
	require.Error(t, err)
}

func TestTheStaffSecretMustDifferFromTheSiteSecretAndBeLongEnough(t *testing.T) {
	lic := licenseFor(t, licensekey.FeatureStaff)

	_, err := ee.New(nil, lic, nil, siteSecret, staff.Config{Secret: siteSecret, SessionTTL: time.Hour, Attempts: attempts})
	require.Error(t, err, "a StaffMember session must never verify as a User's")

	_, err = ee.New(nil, lic, nil, siteSecret, staff.Config{Secret: "short", SessionTTL: time.Hour, Attempts: attempts})
	require.Error(t, err)

	e, err := ee.New(nil, lic, nil, siteSecret, staff.Config{Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour, Attempts: attempts})
	require.NoError(t, err)
	assert.NotNil(t, e.Staff(nil, nil))
}

func TestTheStaffLoginCounterIsRequiredWithTheStaffLicense(t *testing.T) {
	lic := licenseFor(t, licensekey.FeatureStaff)

	_, err := ee.New(nil, lic, nil, siteSecret, staff.Config{Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour})

	require.Error(t, err, "a missing counter must not silently leave the login unthrottled")
}

func TestTheEditionRegistersTheAbuseDetectorNextToTheOtherJobs(t *testing.T) {
	e, err := ee.New(nil, licenseFor(t, licensekey.FeatureStaff), nil, siteSecret, staff.Config{Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour, Attempts: attempts})
	require.NoError(t, err)

	jobs := e.Jobs(nil, nil)

	assert.Len(t, jobs.PeriodicJobs, 3, "retention, usage and the abuse detector")
	assert.NotPanics(t, func() { jobs.Workers(river.NewWorkers()) })
}
