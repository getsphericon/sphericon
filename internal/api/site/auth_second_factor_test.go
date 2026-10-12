package site_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const secondStepPath = "/site/auth/second-factor"

type loginRes struct {
	Outcome   string `json:"outcome"`
	Challenge string `json:"challenge"`
}

// twoStepEnv is a test env on a clock the test moves, for Sam (an active Second factor).
type twoStepEnv struct {
	env   *testhelper.TestEnv
	clock *movableClock
}

func newTwoStepEnv(t *testing.T, opts ...testhelper.Option) *twoStepEnv {
	t.Helper()
	c := &movableClock{t: time.Now()}
	return &twoStepEnv{env: testhelper.Setup(t, append([]testhelper.Option{testhelper.WithClock(c.now)}, opts...)...), clock: c}
}

// challenge passes Sam's password step and returns the challenge it answers with.
func (s *twoStepEnv) challenge(t *testing.T) string {
	t.Helper()
	rec := postLogin(t, s.env, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res loginRes
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.Equal(t, "challenge", res.Outcome)
	require.NotEmpty(t, res.Challenge)
	return res.Challenge
}

// code is Sam's authenticator code at the env clock.
func (s *twoStepEnv) code(t *testing.T) string {
	t.Helper()
	c, err := totp.GenerateCode(fixtures.SecondFactorSamTotpSecret, s.clock.now())
	require.NoError(t, err)
	return c
}

func (s *twoStepEnv) secondStep(t *testing.T, challenge, code string) *httptest.ResponseRecorder {
	t.Helper()
	return sendWithSession(t, s.env, http.MethodPost, secondStepPath, "",
		fmt.Sprintf(`{"challenge":%q,"code":%q}`, challenge, code))
}

func TestPasswordAloneGivesAUserWithASecondFactorAChallengeAndNoSession(t *testing.T) {
	s := newTwoStepEnv(t)
	rec := postLogin(t, s.env, fixtures.SecondFactorSamEmail, fixtures.SecondFactorSamPassword)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Nil(t, sessionCookie(rec.Result()), "no session before the second step")
	var res loginRes
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.Equal(t, "challenge", res.Outcome)
	assert.NotEmpty(t, res.Challenge)
}

func TestTheSecondStepWithATOTPCodeStartsASessionAndIsAudited(t *testing.T) {
	s := newTwoStepEnv(t)
	rec := s.secondStep(t, s.challenge(t), s.code(t))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"outcome":"session"}`, rec.Body.String())
	cookie := sessionCookie(rec.Result())
	require.NotNil(t, cookie)
	assert.Equal(t, http.StatusOK, workspacesStatus(t, s.env, cookie.Value), "the cookie authorizes /site")

	assert.Len(t, entriesNamed(t, s.env, fixtures.OwnerJaneEmail, fixtures.GlobexSlug, events.ActionUserLogin), 1)
}

func TestTheSecondStepRejectsAWrongCodeWithoutASession(t *testing.T) {
	s := newTwoStepEnv(t)
	rec := s.secondStep(t, s.challenge(t), "000000")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, sessionCookie(rec.Result()))
}

func TestAReplayedTOTPCodeIsRejected(t *testing.T) {
	s := newTwoStepEnv(t)
	code := s.code(t)
	require.Equal(t, http.StatusOK, s.secondStep(t, s.challenge(t), code).Code)

	rec := s.secondStep(t, s.challenge(t), code)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, sessionCookie(rec.Result()))
}

func TestARecoveryCodeWorksOnceAndIsAudited(t *testing.T) {
	s := newTwoStepEnv(t)
	require.Equal(t, http.StatusOK, s.secondStep(t, s.challenge(t), fixtures.SecondFactorSamRecoveryCode).Code)
	assert.Equal(t, http.StatusUnauthorized, s.secondStep(t, s.challenge(t), fixtures.SecondFactorSamRecoveryCode).Code)
	assert.Equal(t, http.StatusUnauthorized, s.secondStep(t, s.challenge(t), fixtures.SecondFactorSamSpentRecoveryCode).Code)

	assert.Len(t, entriesNamed(t, s.env, fixtures.OwnerJaneEmail, fixtures.GlobexSlug, events.ActionUserRecoveryCodeUse), 1)
}

func TestAnExpiredChallengeIsRejected(t *testing.T) {
	s := newTwoStepEnv(t)
	challenge := s.challenge(t)
	s.clock.t = s.clock.t.Add(5*time.Minute + time.Second)
	assert.Equal(t, http.StatusUnauthorized, s.secondStep(t, challenge, s.code(t)).Code)
}

func TestAChallengeWorksOnlyOnce(t *testing.T) {
	t.Run("after a TOTP success", func(t *testing.T) {
		s := newTwoStepEnv(t)
		challenge := s.challenge(t)
		require.Equal(t, http.StatusOK, s.secondStep(t, challenge, s.code(t)).Code)
		s.clock.t = s.clock.t.Add(time.Minute) // a fresh, never-used code
		assert.Equal(t, http.StatusUnauthorized, s.secondStep(t, challenge, s.code(t)).Code)
	})
	t.Run("after a Recovery code success", func(t *testing.T) {
		s := newTwoStepEnv(t)
		challenge := s.challenge(t)
		require.Equal(t, http.StatusOK, s.secondStep(t, challenge, fixtures.SecondFactorSamRecoveryCode).Code)
		assert.Equal(t, http.StatusUnauthorized, s.secondStep(t, challenge, s.code(t)).Code)
	})
}

func TestAForgedChallengeIsRejected(t *testing.T) {
	s := newTwoStepEnv(t)
	assert.Equal(t, http.StatusUnauthorized, s.secondStep(t, "not-a-challenge", s.code(t)).Code)
}

func TestUsersWithoutASecondFactorKeepTheOneStepLogin(t *testing.T) {
	s := newTwoStepEnv(t)
	rec := postLogin(t, s.env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"outcome":"session"}`, rec.Body.String())
	assert.NotNil(t, sessionCookie(rec.Result()))
	assert.Len(t, entriesNamed(t, s.env, fixtures.OwnerJohnEmail, fixtures.AcmeSlug, events.ActionUserLogin), 1)
}
