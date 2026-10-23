package staff_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/authattempt"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const loginFailures = 5

func throttledHarness(t *testing.T, ipLimit int) *harness {
	t.Helper()
	return newHarness(t, testhelper.WithRateLimits(config.RateLimits{LoginFailures: loginFailures, LoginIP: ipLimit}))
}

func (h *harness) rawPasswordStep(t *testing.T, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	return post(t, h.env.Server, "/staff/auth/login", `{"email":"`+email+`","password":"`+password+`"}`)
}

func (h *harness) rawSecondStep(t *testing.T, challenge, code string) *httptest.ResponseRecorder {
	t.Helper()
	return post(t, h.env.Server, "/staff/auth/second-factor", `{"challenge":"`+challenge+`","code":"`+code+`"}`)
}

func (h *harness) failPasswords(t *testing.T, email string, times int) {
	t.Helper()
	for i := range times {
		require.Equal(t, http.StatusUnauthorized, h.rawPasswordStep(t, email, "wrong-password").Code, "failure %d", i+1)
	}
}

func requireThrottled(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	assert.Equal(t, "0", rec.Header().Get("X-Ratelimit-Remaining"))
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Empty(t, rec.Header().Get("Set-Cookie"))
}

func TestRepeatedWrongPasswordsAreRefusedEvenForTheRightPassword(t *testing.T) {
	h := throttledHarness(t, 0)
	h.failPasswords(t, fixtures.StaffEnrolledEmail, loginFailures)

	requireThrottled(t, h.rawPasswordStep(t, fixtures.StaffEnrolledEmail, "wrong-password"))
	requireThrottled(t, h.rawPasswordStep(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword))
}

func TestWrongTOTPCodesAreRefusedEvenForTheRightCode(t *testing.T) {
	h := throttledHarness(t, 0)
	challenge := h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword).Challenge
	for i := range loginFailures {
		require.Equal(t, http.StatusUnauthorized, h.rawSecondStep(t, challenge, "000000").Code, "failure %d", i+1)
	}

	requireThrottled(t, h.rawSecondStep(t, challenge, h.code(t, fixtures.StaffEnrolledTotpSecret)))
	requireThrottled(t, h.rawPasswordStep(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword))
}

// A Staff member who is also a customer has two identities: the Staff member's failures
// count under their own kind and never throttle the User of the same address.
func TestStaffFailuresNeverCountAgainstTheUserOfTheSameAddress(t *testing.T) {
	h := throttledHarness(t, 0)
	h.failPasswords(t, fixtures.StaffAlsoCustomerEmail, loginFailures)

	rows, err := h.env.DB.AuthAttempt.Query().Where(authattempt.Email(fixtures.StaffAlsoCustomerEmail)).All(t.Context())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, authattempt.KindStaffLogin, rows[0].Kind)

	rec := post(t, h.env.Server, "/site/auth/login",
		`{"email":"`+fixtures.OwnerJohnEmail+`","password":"`+fixtures.OwnerJohnPassword+`"}`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestTheStaffLoginSharesThePerIPLoginCap(t *testing.T) {
	h := throttledHarness(t, 2)
	for range 2 {
		require.Equal(t, http.StatusUnauthorized, h.rawPasswordStep(t, "nobody@sphericon.test", "x").Code)
	}

	rec := h.rawPasswordStep(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestWithoutLimitsNothingIsThrottled(t *testing.T) {
	h := newHarness(t)
	h.failPasswords(t, fixtures.StaffEnrolledEmail, loginFailures*2)

	assert.Equal(t, staffapi.StaffLoginOutcomeChallenge, h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword).Outcome)
}

// A User's failures count under the User's kind and never throttle the Staff member of
// the same address.
func TestUserFailuresNeverCountAgainstTheStaffMemberOfTheSameAddress(t *testing.T) {
	h := throttledHarness(t, 0)
	for range loginFailures {
		rec := post(t, h.env.Server, "/site/auth/login",
			`{"email":"`+fixtures.OwnerJohnEmail+`","password":"wrong-password"}`)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}

	res := h.challenge(t, fixtures.StaffAlsoCustomerEmail, fixtures.StaffAlsoCustomerPassword)

	assert.Equal(t, staffapi.StaffLoginOutcomeChallenge, res.Outcome)
}

// brokenCounter is a failure counter that lets everything through and cannot record.
type brokenCounter struct{}

var errCounterDown = errors.New("counter down")

func (brokenCounter) Delay(context.Context, string) (time.Duration, error) { return 0, nil }
func (brokenCounter) RecordFailure(context.Context, string) error          { return errCounterDown }
func (brokenCounter) RecordSuccess(context.Context, string) error          { return nil }
func (brokenCounter) Limit() int                                           { return loginFailures }
func (brokenCounter) Now() time.Time                                       { return time.Now() }

var _ login.Throttle = brokenCounter{}

// A counter that cannot record a failure refuses the attempt, never answers a plain 401
// that would let guessing go on uncounted.
func TestABrokenFailureCounterRefusesTheStaffLoginAttempt(t *testing.T) {
	h := newHarness(t)
	h.env.StaffMembers.UseThrottle(brokenCounter{})

	rec := h.rawPasswordStep(t, fixtures.StaffEnrolledEmail, "wrong-password")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, rec.Header().Get("Set-Cookie"))
}
