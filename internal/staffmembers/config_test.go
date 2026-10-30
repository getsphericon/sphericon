package staffmembers_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/staffmembers"
)

var (
	siteSecret = rand.Text() + rand.Text()
	attempts   = accounts.NewAttempts(nil)
)

func TestWithoutASecretTheStaffSignInIsOff(t *testing.T) {
	m, err := staffmembers.New(nil, nil, []string{siteSecret}, staffmembers.Config{})

	require.NoError(t, err, "an Instance needs no Staff member secret to start")
	assert.Nil(t, m.Sessions(), "and then issues no Staff member session")
}

func TestTheStaffSecretMustDifferFromTheSiteSecretAndBeLongEnough(t *testing.T) {
	_, err := staffmembers.New(nil, nil, []string{siteSecret}, staffmembers.Config{Secret: siteSecret, SessionTTL: time.Hour, Attempts: attempts})
	require.Error(t, err, "a StaffMember session must never verify as a User's")

	_, err = staffmembers.New(nil, nil, []string{siteSecret}, staffmembers.Config{Secret: "short", SessionTTL: time.Hour, Attempts: attempts})
	require.Error(t, err)

	secondary := rand.Text() + rand.Text()
	_, err = staffmembers.New(nil, nil, []string{siteSecret, secondary}, staffmembers.Config{Secret: secondary, SessionTTL: time.Hour, Attempts: attempts})
	require.Error(t, err, "a rotation's secondary JWT secret verifies User sessions too")

	m, err := staffmembers.New(nil, nil, []string{siteSecret}, staffmembers.Config{Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour, Attempts: attempts})
	require.NoError(t, err)
	assert.NotNil(t, m.Sessions())
}

func TestTheStaffLoginCounterIsRequiredWithASecret(t *testing.T) {
	_, err := staffmembers.New(nil, nil, []string{siteSecret}, staffmembers.Config{Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour})

	require.Error(t, err, "a missing counter must not silently leave the login unthrottled")
}
