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
	"github.com/getsphericon/sphericon/ee/usecase/staff"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
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
	m, err := staffmembers.New(nil, nil, []string{rand.Text() + rand.Text()}, staffmembers.Config{Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour, Attempts: throttle.NewAttempts(nil)})
	require.NoError(t, err)
	return m.Sessions()
}

func TestTheStaffLicenseNeedsTheStaffSignIn(t *testing.T) {
	_, err := ee.New(ee.Deps{License: licenseFor(t, licensekey.FeatureStaff)}, staff.Config{})

	require.Error(t, err, "the Enterprise sections take the Staff member's sessions")
}

func TestOnlyTheStaffLicenseServesTheEnterpriseSections(t *testing.T) {
	plain, err := ee.New(ee.Deps{License: licenseFor(t, licensekey.FeatureAudit), StaffSessions: staffSessions(t)}, staff.Config{})
	require.NoError(t, err)
	off, err := ee.New(ee.Deps{License: licenseFor(t, licensekey.FeatureAudit)}, staff.Config{})
	require.NoError(t, err, "an instance without the staff license needs no Staff sign-in")
	licensed, err := ee.New(ee.Deps{License: licenseFor(t, licensekey.FeatureStaff), StaffSessions: staffSessions(t)}, staff.Config{})
	require.NoError(t, err)

	assert.Nil(t, plain.StaffSections())
	assert.Nil(t, off.StaffSections())
	assert.NotNil(t, licensed.StaffSections())
}

func TestTheEditionRegistersTheAbuseDetectorNextToTheOtherJobs(t *testing.T) {
	e, err := ee.New(ee.Deps{License: licenseFor(t, licensekey.FeatureStaff), StaffSessions: staffSessions(t)}, staff.Config{})
	require.NoError(t, err)

	jobs := e.Jobs()

	assert.Len(t, jobs.PeriodicJobs, 3, "retention, usage and the abuse detector")
	assert.NotPanics(t, func() { jobs.Workers(river.NewWorkers()) })
}
