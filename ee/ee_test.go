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
	"github.com/getsphericon/sphericon/internal/staffmembers"
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

// staffSessions are the sessions of a Staff sign-in that is on.
func staffSessions(t *testing.T) *staffmembers.Sessions {
	t.Helper()
	m, err := staffmembers.New(nil, nil, []string{rand.Text() + rand.Text()}, staffmembers.Config{Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour, Attempts: accounts.NewAttempts(nil)})
	require.NoError(t, err)
	return m.Sessions()
}

func TestTheStaffLicenseNeedsTheStaffSignIn(t *testing.T) {
	_, err := ee.New(nil, nil, licenseFor(t, licensekey.FeatureStaff), nil, staff.Config{})

	require.Error(t, err, "the Enterprise sections take the Staff member's sessions")
}

func TestOnlyTheStaffLicenseServesTheEnterpriseSections(t *testing.T) {
	plain, err := ee.New(nil, nil, licenseFor(t, licensekey.FeatureAudit), staffSessions(t), staff.Config{})
	require.NoError(t, err)
	off, err := ee.New(nil, nil, licenseFor(t, licensekey.FeatureAudit), nil, staff.Config{})
	require.NoError(t, err, "an instance without the staff license needs no Staff sign-in")
	licensed, err := ee.New(nil, nil, licenseFor(t, licensekey.FeatureStaff), staffSessions(t), staff.Config{})
	require.NoError(t, err)

	assert.Nil(t, plain.StaffSections(nil, nil))
	assert.Nil(t, off.StaffSections(nil, nil))
	assert.NotNil(t, licensed.StaffSections(nil, nil))
}

func TestTheEditionRegistersTheAbuseDetectorNextToTheOtherJobs(t *testing.T) {
	e, err := ee.New(nil, nil, licenseFor(t, licensekey.FeatureStaff), staffSessions(t), staff.Config{})
	require.NoError(t, err)

	jobs := e.Jobs(nil, nil)

	assert.Len(t, jobs.PeriodicJobs, 3, "retention, usage and the abuse detector")
	assert.NotPanics(t, func() { jobs.Workers(river.NewWorkers()) })
}
