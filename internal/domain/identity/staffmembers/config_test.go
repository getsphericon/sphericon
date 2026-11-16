package staffmembers_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
)

var (
	siteSecret = rand.Text() + rand.Text()
	attempts   = throttle.NewAttempts(nil, time.Now)
)

func TestWithoutASecretTheStaffSignInIsOff(t *testing.T) {
	cfg := staffmembers.Config{Clock: time.Now}

	require.NoError(t, cfg.Validate([]string{siteSecret}), "an Instance needs no Staff member secret to start")
	assert.Nil(t, staffmembers.New(nil, nil, cfg).Sessions(), "and then issues no Staff member session")
}

func TestTheStaffSecretMustDifferFromTheSiteSecretAndBeLongEnough(t *testing.T) {
	err := staffmembers.Config{Clock: time.Now, Secret: siteSecret, SessionTTL: time.Hour, Attempts: attempts}.Validate([]string{siteSecret})
	require.Error(t, err, "a StaffMember session must never verify as a User's")

	err = staffmembers.Config{Clock: time.Now, Secret: "short", SessionTTL: time.Hour, Attempts: attempts}.Validate([]string{siteSecret})
	require.Error(t, err)

	secondary := rand.Text() + rand.Text()
	err = staffmembers.Config{Clock: time.Now, Secret: secondary, SessionTTL: time.Hour, Attempts: attempts}.Validate([]string{siteSecret, secondary})
	require.Error(t, err, "a rotation's secondary JWT secret verifies User sessions too")

	cfg := staffmembers.Config{Clock: time.Now, Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour, Attempts: attempts}
	require.NoError(t, cfg.Validate([]string{siteSecret}))
	assert.NotNil(t, staffmembers.New(nil, nil, cfg).Sessions())
}

func TestTheStaffLoginCounterIsRequiredWithASecret(t *testing.T) {
	err := staffmembers.Config{Clock: time.Now, Secret: rand.Text() + rand.Text(), SessionTTL: time.Hour}.Validate([]string{siteSecret})

	require.Error(t, err, "a missing counter must not silently leave the login unthrottled")
}

func TestTheStaffMemberNeedsTheInstanceClock(t *testing.T) {
	require.Error(t, staffmembers.Config{}.Validate([]string{siteSecret}), "the module reads the Instance clock, sign-in on or off")
}
