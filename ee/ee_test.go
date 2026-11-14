package ee_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee"
	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// staffSessions are the sessions of a Staff sign-in that is on.
func staffSessions(t *testing.T) *staffmembers.Sessions {
	t.Helper()
	m, err := staffmembers.New(nil, nil, []string{rand.Text() + rand.Text()}, staffmembers.Config{Clock: time.Now, Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour, Attempts: throttle.NewAttempts(nil, time.Now)})
	require.NoError(t, err)
	return m.Sessions()
}

func TestTheStaffLicenseNeedsTheStaffSignIn(t *testing.T) {
	_, err := ee.New(ee.Deps{License: testhelper.License(t, licensekey.FeatureStaff)}, workspaceoversight.Config{Clock: time.Now})

	require.Error(t, err, "the Enterprise sections take the Staff member's sessions")
}

func TestOnlyTheStaffLicenseServesTheEnterpriseSections(t *testing.T) {
	plain, err := ee.New(ee.Deps{License: testhelper.License(t, licensekey.FeatureAudit), StaffSessions: staffSessions(t)}, workspaceoversight.Config{Clock: time.Now})
	require.NoError(t, err)
	off, err := ee.New(ee.Deps{License: testhelper.License(t, licensekey.FeatureAudit)}, workspaceoversight.Config{Clock: time.Now})
	require.NoError(t, err, "an instance without the staff license needs no Staff sign-in")
	licensed, err := ee.New(ee.Deps{License: testhelper.License(t, licensekey.FeatureStaff), StaffSessions: staffSessions(t)}, workspaceoversight.Config{Clock: time.Now})
	require.NoError(t, err)

	assert.Nil(t, plain.StaffSections())
	assert.Nil(t, off.StaffSections())
	assert.NotNil(t, licensed.StaffSections())
}
