package operator_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	operatorapi "github.com/getsphericon/sphericon/gen/operator"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// movableClock is the clock the server checks TOTP codes and session expiry against.
type movableClock struct{ t time.Time }

func (c *movableClock) now() time.Time { return c.t }

type harness struct {
	env   *testhelper.TestEnv
	clock *movableClock
}

func newHarness(t *testing.T, opts ...testhelper.Option) *harness {
	t.Helper()
	c := &movableClock{t: time.Now()}
	return &harness{env: testhelper.Setup(t, append([]testhelper.Option{testhelper.WithClock(c.now)}, opts...)...), clock: c}
}

// code is the authenticator code of secret at the env clock.
func (h *harness) code(t *testing.T, secret string) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, h.clock.now())
	require.NoError(t, err)
	return c
}

// passwordStep runs the first login step and returns its answer.
func (h *harness) passwordStep(t *testing.T, email, password string) operatorapi.OperatorAuthLoginRes {
	t.Helper()
	res, err := h.env.OperatorAnonymous(t).OperatorAuthLogin(t.Context(), &operatorapi.OperatorLoginInput{Email: email, Password: password})
	require.NoError(t, err)
	return res
}

// challenge passes the password step and returns the challenge it answers with.
func (h *harness) challenge(t *testing.T, email, password string) *operatorapi.OperatorLoginResult {
	t.Helper()
	res, ok := h.passwordStep(t, email, password).(*operatorapi.OperatorLoginResult)
	require.True(t, ok, "the password step grants a challenge, got %T", res)
	return res
}

func (h *harness) secondStep(t *testing.T, challenge, code string) operatorapi.OperatorAuthSecondFactorRes {
	t.Helper()
	res, err := h.env.OperatorAnonymous(t).OperatorAuthSecondFactor(t.Context(), &operatorapi.OperatorSecondFactorInput{Challenge: challenge, Code: code})
	require.NoError(t, err)
	return res
}

// session runs the whole two-step login of an enrolled Operator and returns the
// session cookie.
func (h *harness) session(t *testing.T, email, password, secret string) *http.Cookie {
	t.Helper()
	res, ok := h.secondStep(t, h.challenge(t, email, password).Challenge, h.code(t, secret)).(*operatorapi.OperatorResourceHeaders)
	require.True(t, ok, "the second step starts a session")
	return parseCookie(t, res.SetCookie)
}

func parseCookie(t *testing.T, setCookie string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	rec.Header().Add("Set-Cookie", setCookie)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

func (h *harness) me(t *testing.T, token string) operatorapi.OperatorMeGetRes {
	t.Helper()
	res, err := h.env.OperatorWithToken(t, token).OperatorMeGet(t.Context())
	require.NoError(t, err)
	return res
}

func requireProblem(t *testing.T, res any, status int) {
	t.Helper()
	switch p := res.(type) {
	case *operatorapi.ProblemDetails:
		assert.EqualValues(t, status, p.Status.Value)
	case *operatorapi.ProblemDetailsHeaders:
		assert.EqualValues(t, status, p.Response.Status.Value)
	default:
		require.Failf(t, "not a problem", "got %T", res)
	}
}

func get(t *testing.T, h http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestWithoutALicenseTheWholeSurfaceIs404(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	for _, path := range []string{"/operator/me", "/operator/auth/second-factor", "/operator/unknown"} {
		rec := get(t, env.Server, path)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.NotContains(t, rec.Header().Get("Content-Type"), "text/html", "%s must not fall through to the SPA shell", path)
	}
	rec := post(t, env.Server, "/operator/auth/login",
		`{"email":"`+fixtures.OperatorEnrolledEmail+`","password":"`+fixtures.OperatorEnrolledPassword+`"}`)
	assert.Equal(t, http.StatusNotFound, rec.Code, "no Operator can log in unlicensed")
}

func TestAnAnonymousRequestToTheLicensedSurfaceIs401(t *testing.T) {
	h := newHarness(t)
	requireProblem(t, h.me(t, ""), http.StatusUnauthorized)
}

func TestAWrongPasswordAndAnUnknownEmailAnswerTheSame401(t *testing.T) {
	h := newHarness(t)

	wrong := h.passwordStep(t, fixtures.OperatorEnrolledEmail, "not-the-password")
	unknown := h.passwordStep(t, "nobody@sphericon.test", fixtures.OperatorEnrolledPassword)

	requireProblem(t, wrong, http.StatusUnauthorized)
	requireProblem(t, unknown, http.StatusUnauthorized)
	assert.Equal(t, unknown, wrong)
}

func TestThePasswordAloneGivesAChallengeAndNoSession(t *testing.T) {
	h := newHarness(t)

	res := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword)

	assert.Equal(t, operatorapi.OperatorLoginOutcomeChallenge, res.Outcome)
	assert.NotEmpty(t, res.Challenge)
	assert.False(t, res.Enrolment.Set, "an enrolled Operator is not shown a secret")
}

func TestTheSecondStepWithATOTPCodeStartsASession(t *testing.T) {
	h := newHarness(t)

	cookie := h.session(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword, fixtures.OperatorEnrolledTotpSecret)

	assert.Equal(t, "OPERATOR_JWT", cookie.Name)
	assert.True(t, cookie.HttpOnly)
	me, ok := h.me(t, cookie.Value).(*operatorapi.OperatorResource)
	require.True(t, ok, "the cookie authorizes the Operator surface")
	assert.EqualValues(t, fixtures.OperatorEnrolledEmail, me.Email)
}

func TestTheSessionIsShorterThanACustomersAndHasNoRememberMe(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword, fixtures.OperatorEnrolledTotpSecret)
	ttl := time.Duration(cookie.MaxAge) * time.Second

	assert.Positive(t, ttl)
	assert.Less(t, ttl, 24*time.Hour, "shorter than SESSION_TTL's default")

	h.clock.t = h.clock.t.Add(ttl + time.Second)
	requireProblem(t, h.me(t, cookie.Value), http.StatusUnauthorized)
}

func TestTheSecondStepRejectsAWrongCodeWithoutASession(t *testing.T) {
	h := newHarness(t)
	challenge := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword).Challenge

	requireProblem(t, h.secondStep(t, challenge, "000000"), http.StatusUnauthorized)
}

func TestAReplayedCodeAndAnExpiredChallengeAreRejected(t *testing.T) {
	h := newHarness(t)
	code := h.code(t, fixtures.OperatorEnrolledTotpSecret)
	first := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword).Challenge
	_, ok := h.secondStep(t, first, code).(*operatorapi.OperatorResourceHeaders)
	require.True(t, ok)

	replay := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword).Challenge
	requireProblem(t, h.secondStep(t, replay, code), http.StatusUnauthorized)
	requireProblem(t, h.secondStep(t, first, h.code(t, fixtures.OperatorEnrolledTotpSecret)), http.StatusUnauthorized)

	h.clock.t = h.clock.t.Add(5*time.Minute + time.Second)
	requireProblem(t, h.secondStep(t, replay, h.code(t, fixtures.OperatorEnrolledTotpSecret)), http.StatusUnauthorized)
}

func TestAFirstLoginEnrolsATOTPAndOnlyTheConfirmingCodeStartsASession(t *testing.T) {
	h := newHarness(t)

	res := h.challenge(t, fixtures.OperatorFreshEmail, fixtures.OperatorFreshPassword)
	assert.Equal(t, operatorapi.OperatorLoginOutcomeEnrolment, res.Outcome)
	require.True(t, res.Enrolment.Set)
	enrolment := res.Enrolment.Value
	assert.NotEmpty(t, enrolment.Secret)
	assert.True(t, strings.HasPrefix(enrolment.URI, "otpauth://totp/"))
	assert.True(t, strings.HasPrefix(enrolment.QrCode, "data:image/png;base64,"))

	requireProblem(t, h.secondStep(t, res.Challenge, "000000"), http.StatusUnauthorized)
	again := h.challenge(t, fixtures.OperatorFreshEmail, fixtures.OperatorFreshPassword)
	assert.Equal(t, operatorapi.OperatorLoginOutcomeEnrolment, again.Outcome, "a wrong code enrols nothing")

	secret := again.Enrolment.Value.Secret
	done, ok := h.secondStep(t, again.Challenge, h.code(t, secret)).(*operatorapi.OperatorResourceHeaders)
	require.True(t, ok)
	_, ok = h.me(t, parseCookie(t, done.SetCookie).Value).(*operatorapi.OperatorResource)
	assert.True(t, ok)

	h.clock.t = h.clock.t.Add(time.Minute)
	next := h.challenge(t, fixtures.OperatorFreshEmail, fixtures.OperatorFreshPassword)
	assert.Equal(t, operatorapi.OperatorLoginOutcomeChallenge, next.Outcome, "enrolled once, challenged from then on")
	assert.False(t, next.Enrolment.Set)
}

func TestLogoutClearsTheCookieAndEndsTheSessionServerSide(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword, fixtures.OperatorEnrolledTotpSecret)
	copied := cookie.Value

	res, err := h.env.OperatorWithToken(t, cookie.Value).OperatorAuthLogout(t.Context())
	require.NoError(t, err)

	out, ok := res.(*operatorapi.OperatorAuthLogoutNoContent)
	require.True(t, ok)
	cleared := parseCookie(t, out.SetCookie)
	assert.Equal(t, "OPERATOR_JWT", cleared.Name)
	assert.Empty(t, cleared.Value)
	assert.Negative(t, cleared.MaxAge)
	requireProblem(t, h.me(t, copied), http.StatusUnauthorized)
}

func TestLogoutNeedsASession(t *testing.T) {
	h := newHarness(t)

	res, err := h.env.OperatorAnonymous(t).OperatorAuthLogout(t.Context())
	require.NoError(t, err)

	requireProblem(t, res, http.StatusUnauthorized)
}

func TestARepeatedPasswordStepKeepsTheSamePendingSecret(t *testing.T) {
	h := newHarness(t)

	first := h.challenge(t, fixtures.OperatorFreshEmail, fixtures.OperatorFreshPassword)
	second := h.challenge(t, fixtures.OperatorFreshEmail, fixtures.OperatorFreshPassword)

	require.True(t, first.Enrolment.Set)
	require.True(t, second.Enrolment.Set)
	assert.Equal(t, first.Enrolment.Value.Secret, second.Enrolment.Value.Secret, "a second password login cannot swap the secret")
	assert.Equal(t, first.Enrolment.Value.URI, second.Enrolment.Value.URI)

	secret := first.Enrolment.Value.Secret
	_, ok := h.secondStep(t, first.Challenge, h.code(t, secret)).(*operatorapi.OperatorResourceHeaders)
	assert.True(t, ok, "the first challenge still confirms the secret the Operator enrolled")
}

func TestARepeatedPasswordStepLeavesAConfirmedOperatorUntouched(t *testing.T) {
	h := newHarness(t)
	before := h.env.DB.Operator.GetX(t.Context(), fixtures.OperatorEnrolledID)

	res := h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword)
	h.challenge(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword)

	assert.False(t, res.Enrolment.Set)
	after := h.env.DB.Operator.GetX(t.Context(), fixtures.OperatorEnrolledID)
	assert.Equal(t, before.TotpSecretEncrypted, after.TotpSecretEncrypted)
	assert.Equal(t, before.TotpConfirmedAt, after.TotpConfirmedAt)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch)
}

func TestAUserSessionNeverOpensTheOperatorSurface(t *testing.T) {
	h := newHarness(t)
	userToken := h.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	requireProblem(t, h.me(t, userToken), http.StatusUnauthorized)
	rec := get(t, h.env.Server, "/operator/me", &http.Cookie{Name: "JWT", Value: userToken})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAnOperatorSessionNeverOpensSite(t *testing.T) {
	h := newHarness(t)
	token := h.session(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword, fixtures.OperatorEnrolledTotpSecret).Value

	for _, name := range []string{"JWT", "OPERATOR_JWT"} {
		rec := get(t, h.env.Server, "/site/workspaces", &http.Cookie{Name: name, Value: token})
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "cookie %s", name)
	}
}

func TestAnOperatorWhoIsAlsoACustomerKeepsTwoIdentities(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, fixtures.OwnerJohnEmail, fixtures.OperatorAlsoCustomerEmail, "the fixture shares an address with a User")

	cookie := h.session(t, fixtures.OperatorAlsoCustomerEmail, fixtures.OperatorAlsoCustomerPassword, fixtures.OperatorAlsoCustomerTotpSecret)
	_, ok := h.me(t, cookie.Value).(*operatorapi.OperatorResource)
	assert.True(t, ok)

	requireProblem(t, h.passwordStep(t, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword), http.StatusUnauthorized)
}

func TestEndingAnOperatorsSessionsRejectsTheOldToken(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword, fixtures.OperatorEnrolledTotpSecret)

	h.env.DB.Operator.UpdateOneID(fixtures.OperatorEnrolledID).AddSessionEpoch(1).ExecX(t.Context())

	requireProblem(t, h.me(t, cookie.Value), http.StatusUnauthorized)
}
