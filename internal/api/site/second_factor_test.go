package site_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// secondFactorEnv is a test env on a clock the test moves, with John signed in.
type secondFactorEnv struct {
	env   *testhelper.TestEnv
	clock *movableClock
}

func newSecondFactorEnv(t *testing.T) *secondFactorEnv {
	t.Helper()
	c := &movableClock{t: time.Now()}
	return &secondFactorEnv{env: testhelper.Setup(t, testhelper.WithClock(c.now)), clock: c}
}

// send sends a JSON site request under the session and decodes a JSON answer into out.
func (s *secondFactorEnv) send(t *testing.T, method, path, session, body string, out any) (status int, raw string) {
	t.Helper()
	rec := sendWithSession(t, s.env, method, path, session, body)
	if out != nil && rec.Code < 300 && rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), out))
	}
	return rec.Code, reissued(t, rec)
}

// code is the authenticator app's current code for the secret.
func (s *secondFactorEnv) code(t *testing.T, secret string) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, s.clock.now())
	require.NoError(t, err)
	return c
}

type factorStatus struct {
	Enabled                bool `json:"enabled"`
	Pending                bool `json:"pending"`
	RecoveryCodesRemaining int  `json:"recoveryCodesRemaining"`
}

func (s *secondFactorEnv) status(t *testing.T, session string) factorStatus {
	t.Helper()
	var st factorStatus
	code, _ := s.send(t, http.MethodGet, "/site/me/second-factor", session, "", &st)
	require.Equal(t, http.StatusOK, code)
	return st
}

type enrollmentRes struct {
	Secret     string `json:"secret"`
	OtpauthURI string `json:"otpauthUri"`
	QRCode     string `json:"qrCode"`
}

func (s *secondFactorEnv) start(t *testing.T, session string) enrollmentRes {
	t.Helper()
	var e enrollmentRes
	code, _ := s.send(t, http.MethodPost, "/site/me/second-factor/enrollment", session, johnPassword, &e)
	require.Equal(t, http.StatusOK, code)
	return e
}

// johnPassword is the body proving John's password.
var johnPassword = `{"currentPassword":"` + fixtures.OwnerJohnPassword + `"}`

// confirmBody is a confirmation request with John's password and code.
func confirmBody(code string) string {
	return `{"currentPassword":"` + fixtures.OwnerJohnPassword + `","code":"` + code + `"}`
}

type recoveryCodesRes struct {
	Codes []string `json:"codes"`
}

// enroll enrolls John and returns the secret, his Recovery codes and the
// reissued acting session.
func (s *secondFactorEnv) enroll(t *testing.T, session string) (secret string, codes []string, fresh string) {
	t.Helper()
	e := s.start(t, session)
	var rc recoveryCodesRes
	status, fresh := s.send(t, http.MethodPost, "/site/me/second-factor/enrollment/confirm", session,
		confirmBody(s.code(t, e.Secret)), &rc)
	require.Equal(t, http.StatusOK, status)
	require.NotEmpty(t, fresh)
	return e.Secret, rc.Codes, fresh
}

func TestEnrollmentIsPendingUntilConfirmedWithAValidCode(t *testing.T) {
	s := newSecondFactorEnv(t)
	other := s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	acting := s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	e := s.start(t, acting)
	assert.NotEmpty(t, e.Secret)
	assert.Contains(t, e.OtpauthURI, "otpauth://totp/")
	assert.Contains(t, e.OtpauthURI, "secret="+e.Secret)
	assert.Contains(t, e.QRCode, "data:image/png;base64,")
	assert.Equal(t, factorStatus{Pending: true}, s.status(t, acting), "an unconfirmed secret is not a Second factor")

	status, fresh := s.send(t, http.MethodPost, "/site/me/second-factor/enrollment/confirm", acting,
		confirmBody(wrong(s.code(t, e.Secret))), nil)
	assert.Equal(t, http.StatusUnprocessableEntity, status, "a wrong code")
	assert.Empty(t, fresh)
	assert.False(t, s.status(t, acting).Enabled, "a wrong code does not activate it")
	assert.Equal(t, http.StatusOK, workspacesStatus(t, s.env, other), "nothing ended yet")

	var rc recoveryCodesRes
	status, fresh = s.send(t, http.MethodPost, "/site/me/second-factor/enrollment/confirm", acting,
		confirmBody(s.code(t, e.Secret)), &rc)
	require.Equal(t, http.StatusOK, status)
	assert.Len(t, rc.Codes, 10)
	assert.Len(t, uniq(rc.Codes), 10)

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, s.env, other), "other sessions end")
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, s.env, acting), "the acting cookie as sent")
	require.NotEmpty(t, fresh)
	assert.Equal(t, http.StatusOK, workspacesStatus(t, s.env, fresh), "the acting session continues")
	assert.Equal(t, factorStatus{Enabled: true, RecoveryCodesRemaining: 10}, s.status(t, fresh))
}

// A hijacked session must not enroll a factor of its own and sign the real User out
// elsewhere: both enrollment steps prove the password.
func TestEnrollmentRequiresThePasswordOnBothSteps(t *testing.T) {
	s := newSecondFactorEnv(t)
	other := s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	acting := s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	wrongPassword := `{"currentPassword":"wrong-password"}`

	status, _ := s.send(t, http.MethodPost, "/site/me/second-factor/enrollment", acting, wrongPassword, nil)
	assert.Equal(t, http.StatusForbidden, status, "starting with a wrong password")
	assert.Equal(t, factorStatus{}, s.status(t, acting), "nothing pending")

	e := s.start(t, acting)
	status, fresh := s.send(t, http.MethodPost, "/site/me/second-factor/enrollment/confirm", acting,
		`{"currentPassword":"wrong-password","code":"`+s.code(t, e.Secret)+`"}`, nil)
	assert.Equal(t, http.StatusForbidden, status, "confirming with a wrong password")
	assert.Empty(t, fresh)
	assert.Equal(t, factorStatus{Pending: true}, s.status(t, acting), "still only pending")
	assert.Equal(t, http.StatusOK, workspacesStatus(t, s.env, other), "no session ends")
}

// wrong is a code that differs from c in its last digit.
func wrong(c string) string {
	return c[:len(c)-1] + string('0'+(c[len(c)-1]-'0'+1)%10)
}

func uniq(xs []string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}

func (s *secondFactorEnv) disable(t *testing.T, session, password, code string) (status int, raw string) {
	t.Helper()
	return s.send(t, http.MethodPost, "/site/me/second-factor/disable", session,
		`{"currentPassword":"`+password+`","code":"`+code+`"}`, nil)
}

func (s *secondFactorEnv) regenerate(t *testing.T, session, password string) (status int, codes []string, raw string) {
	t.Helper()
	var rc recoveryCodesRes
	status, fresh := s.send(t, http.MethodPost, "/site/me/second-factor/recovery-codes", session,
		`{"currentPassword":"`+password+`"}`, &rc)
	return status, rc.Codes, fresh
}

func TestAnActiveSecondFactorCannotBeReplacedByStartingAgain(t *testing.T) {
	s := newSecondFactorEnv(t)
	_, _, session := s.enroll(t, s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil))

	status, _ := s.send(t, http.MethodPost, "/site/me/second-factor/enrollment", session, johnPassword, nil)
	assert.Equal(t, http.StatusConflict, status)
	assert.True(t, s.status(t, session).Enabled)
}

func TestRegeneratingRecoveryCodesInvalidatesThePreviousSet(t *testing.T) {
	s := newSecondFactorEnv(t)
	_, first, session := s.enroll(t, s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil))
	other := s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	status, _, fresh := s.regenerate(t, session, "wrong-password")
	assert.Equal(t, http.StatusForbidden, status, "the password is required")
	assert.Empty(t, fresh)

	status, second, fresh := s.regenerate(t, session, fixtures.OwnerJohnPassword)
	require.Equal(t, http.StatusOK, status)
	assert.Len(t, second, 10)
	assert.NotContains(t, second, first[0])
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, s.env, other), "other sessions end")
	require.NotEmpty(t, fresh)
	assert.Equal(t, http.StatusOK, workspacesStatus(t, s.env, fresh), "the acting session continues")
	session = fresh

	status, _ = s.disable(t, session, fixtures.OwnerJohnPassword, first[0])
	assert.Equal(t, http.StatusUnprocessableEntity, status, "a code of the previous set")
	status, _ = s.disable(t, session, fixtures.OwnerJohnPassword, second[0])
	assert.Equal(t, http.StatusNoContent, status, "a code of the new set")
}

func TestDisableRequiresThePasswordAndACurrentCode(t *testing.T) {
	s := newSecondFactorEnv(t)
	secret, _, session := s.enroll(t, s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil))
	other := s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	s.clock.t = s.clock.t.Add(30 * time.Second)
	status, fresh := s.disable(t, session, "wrong-password", s.code(t, secret))
	assert.Equal(t, http.StatusForbidden, status, "a wrong password")
	assert.Empty(t, fresh)
	status, _ = s.disable(t, session, fixtures.OwnerJohnPassword, wrong(s.code(t, secret)))
	assert.Equal(t, http.StatusUnprocessableEntity, status, "a wrong code")
	assert.True(t, s.status(t, session).Enabled)
	assert.Equal(t, http.StatusOK, workspacesStatus(t, s.env, other), "a refused disable ends nothing")

	status, fresh = s.disable(t, session, fixtures.OwnerJohnPassword, s.code(t, secret))
	require.Equal(t, http.StatusNoContent, status)
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, s.env, other), "other sessions end")
	require.NotEmpty(t, fresh)
	assert.Equal(t, http.StatusOK, workspacesStatus(t, s.env, fresh), "the acting session continues")
	assert.Equal(t, factorStatus{}, s.status(t, fresh))

	status, _ = s.disable(t, fresh, fixtures.OwnerJohnPassword, s.code(t, secret))
	assert.Equal(t, http.StatusConflict, status, "nothing left to disable")
}

func TestATOTPCodeWorksOnce(t *testing.T) {
	s := newSecondFactorEnv(t)
	secret, _, session := s.enroll(t, s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil))

	status, _ := s.disable(t, session, fixtures.OwnerJohnPassword, s.code(t, secret))
	assert.Equal(t, http.StatusUnprocessableEntity, status, "the confirmation code replayed")

	s.clock.t = s.clock.t.Add(30 * time.Second)
	status, _ = s.disable(t, session, fixtures.OwnerJohnPassword, s.code(t, secret))
	assert.Equal(t, http.StatusNoContent, status, "the next step's code")
}

func TestRecoveryCodesAreNotRetrievableLater(t *testing.T) {
	s := newSecondFactorEnv(t)
	_, codes, session := s.enroll(t, s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil))

	for _, path := range []string{"/site/me/second-factor", "/site/me"} {
		rec := sendWithSession(t, s.env, http.MethodGet, path, session, "")
		require.Equal(t, http.StatusOK, rec.Code)
		for _, c := range codes {
			assert.NotContains(t, rec.Body.String(), c, path)
		}
	}
}

func TestEnrollRegenerateAndDisableAreAuditedWithoutSecrets(t *testing.T) {
	s := newSecondFactorEnv(t)
	secret, codes, session := s.enroll(t, s.env.SiteToken(t, fixtures.OwnerJohnEmail, nil))
	status, regenerated, session := s.regenerate(t, session, fixtures.OwnerJohnPassword)
	require.Equal(t, http.StatusOK, status)
	status, _ = s.disable(t, session, fixtures.OwnerJohnPassword, regenerated[0])
	require.Equal(t, http.StatusNoContent, status)

	for _, action := range []string{
		events.ActionUserSecondFactorEnroll,
		events.ActionUserRecoveryCodesRegenerate,
		events.ActionUserRecoveryCodeUse,
		events.ActionUserSecondFactorDisable,
	} {
		got := entriesNamed(t, s.env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, action)
		require.Lenf(t, got, 1, action)
		assert.Equal(t, "John", got[0].Actor.Name.Value)
		assert.Equal(t, "user", got[0].Target.Type)
		assert.False(t, got[0].Diff.IsSet() && !got[0].Diff.IsNull(), "no diff: nothing secret to show")
		raw := fmt.Sprintf("%+v", got[0])
		assert.NotContains(t, raw, secret)
		for _, c := range append(codes, regenerated...) {
			assert.NotContains(t, raw, c)
		}
	}
}
