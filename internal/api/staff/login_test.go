package staff_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The core /staff surface runs on every Instance without a License key (ADR 0063), so every
// test here runs unlicensed unless it says otherwise.

// movableClock is the clock the server checks TOTP codes and session expiry against.
type movableClock struct{ t time.Time }

func (c *movableClock) now() time.Time { return c.t }

type harness struct {
	env   *testhelper.TestEnv
	clock *movableClock
}

// newHarness is a self-hosted Instance without a License key (a saas one refuses to start
// without the `staff` feature).
func newHarness(t *testing.T, opts ...testhelper.Option) *harness {
	t.Helper()
	return newLicensedHarness(t, append([]testhelper.Option{testhelper.WithoutLicense(), testhelper.WithMode(config.ModeSelfHosted)}, opts...)...)
}

// newLicensedHarness is an Instance licensed for every Enterprise feature.
func newLicensedHarness(t *testing.T, opts ...testhelper.Option) *harness {
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
func (h *harness) passwordStep(t *testing.T, email, password string) (*staffapi.StaffLoginResult, error) {
	t.Helper()
	return h.env.StaffAnonymous(t).StaffAuthLogin(t.Context(), &staffapi.StaffLoginInput{Email: email, Password: password})
}

// challenge passes the password step and returns the challenge it answers with.
func (h *harness) challenge(t *testing.T, email, password string) *staffapi.StaffLoginResult {
	t.Helper()
	res, err := h.passwordStep(t, email, password)
	require.NoError(t, err, "the password step grants a challenge")
	return res
}

func (h *harness) secondStep(t *testing.T, challenge, code string) (*staffapi.StaffResourceHeaders, error) {
	t.Helper()
	return h.env.StaffAnonymous(t).StaffAuthSecondFactor(t.Context(), &staffapi.StaffSecondFactorInput{Challenge: challenge, Code: code})
}

// session runs the whole two-step login of an enrolled Staff member and returns the
// session cookie.
func (h *harness) session(t *testing.T, email, password, secret string) *http.Cookie {
	t.Helper()
	res, err := h.secondStep(t, h.challenge(t, email, password).Challenge, h.code(t, secret))
	require.NoError(t, err, "the second step starts a session")
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

func (h *harness) me(t *testing.T, token string) (*staffapi.StaffResource, error) {
	t.Helper()
	return h.env.StaffWithToken(t, token).StaffMeGet(t.Context())
}

// errOf is the error of a call's two results, for a call that is expected to be refused.
func errOf[T any](_ T, err error) error {
	return err
}

// requireProblem fails unless err is a refusal with status and a problem body carrying it, and
// returns the problem.
func requireProblem(t *testing.T, err error, status int) staffapi.ProblemDetails {
	t.Helper()
	problem := testhelper.StaffRefusal(t, err, status)
	assert.EqualValues(t, status, problem.Status.Value)
	return problem
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

// ADR 0063: `sphericon staff create` on a self-hosted Instance with no License key makes a Staff
// member who enrols a TOTP, signs in and reads who they are.
func TestASelfhostedInstanceWithoutALicenseSignsACreatedStaffMemberIn(t *testing.T) {
	h := newHarness(t)
	password, err := h.env.StaffMembers.Create(t.Context(), "ops@selfhosted.test")
	require.NoError(t, err)

	res := h.challenge(t, "ops@selfhosted.test", password)
	require.Equal(t, staffapi.StaffLoginOutcomeEnrolment, res.Outcome)
	done, err := h.secondStep(t, res.Challenge, h.code(t, res.Enrolment.Value.Secret))
	require.NoError(t, err, "the confirming code starts a session")

	me, err := h.me(t, parseCookie(t, done.SetCookie).Value)
	require.NoError(t, err, "the core surface answers without a license")
	assert.EqualValues(t, "ops@selfhosted.test", me.Email)
}

func TestWithoutALicenseTheEnterpriseRoutesAnswer404AndTheCoreOnesAnswer(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)

	for _, path := range []string{"/staff/workspaces", "/staff/workspaces/" + fixtures.AcmePublicID, "/staff/workspaces/" + fixtures.AcmePublicID + "/audit-entries", "/staff/signup-survey", "/staff/unknown"} {
		rec := get(t, h.env.Server, path, cookie)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.NotContains(t, rec.Header().Get("Content-Type"), "text/html", "%s must not fall through to the SPA shell", path)
	}
	assert.Equal(t, http.StatusOK, get(t, h.env.Server, "/staff/me", cookie).Code)
}

func TestWithTheLicenseTheEnterpriseRoutesNeedAStaffSession(t *testing.T) {
	h := newLicensedHarness(t)
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)

	anonymous := get(t, h.env.Server, "/staff/workspaces")
	signedIn := get(t, h.env.Server, "/staff/workspaces", cookie)

	assert.Equal(t, http.StatusUnauthorized, anonymous.Code)
	assert.Equal(t, http.StatusOK, signedIn.Code)
}

func TestWithoutAStaffSecretTheWholeSurfaceIs404(t *testing.T) {
	h := newHarness(t, testhelper.WithoutStaffSecret())

	for _, path := range []string{"/staff/me", "/staff/auth/second-factor", "/staff/instance", "/staff/workspaces"} {
		rec := get(t, h.env.Server, path)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.NotContains(t, rec.Header().Get("Content-Type"), "text/html", "%s must not fall through to the SPA shell", path)
	}
	rec := post(t, h.env.Server, "/staff/auth/login",
		`{"email":"`+fixtures.StaffEnrolledEmail+`","password":"`+fixtures.StaffEnrolledPassword+`"}`)
	assert.Equal(t, http.StatusNotFound, rec.Code, "no StaffMember can sign in without the secret")
}

func TestAnAnonymousRequestToTheSurfaceIs401(t *testing.T) {
	h := newHarness(t)
	requireProblem(t, errOf(h.me(t, "")), http.StatusUnauthorized)
}

func TestAWrongPasswordAndAnUnknownEmailAnswerTheSame401(t *testing.T) {
	h := newHarness(t)

	_, wrongErr := h.passwordStep(t, fixtures.StaffEnrolledEmail, "not-the-password")
	_, unknownErr := h.passwordStep(t, "nobody@sphericon.test", fixtures.StaffEnrolledPassword)

	wrong := requireProblem(t, wrongErr, http.StatusUnauthorized)
	unknown := requireProblem(t, unknownErr, http.StatusUnauthorized)
	assert.Equal(t, unknown, wrong)
}

func TestThePasswordAloneGivesAChallengeAndNoSession(t *testing.T) {
	h := newHarness(t)

	res := h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword)

	assert.Equal(t, staffapi.StaffLoginOutcomeChallenge, res.Outcome)
	assert.NotEmpty(t, res.Challenge)
	assert.False(t, res.Enrolment.Set, "an enrolled StaffMember is not shown a secret")
}

func TestTheSecondStepWithATOTPCodeStartsASession(t *testing.T) {
	h := newHarness(t)

	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)

	assert.Equal(t, "STAFF_JWT", cookie.Name)
	assert.True(t, cookie.HttpOnly)
	me, err := h.me(t, cookie.Value)
	require.NoError(t, err, "the cookie authorizes the StaffMember surface")
	assert.EqualValues(t, fixtures.StaffEnrolledEmail, me.Email)
}

func TestTheSessionIsShorterThanACustomersAndHasNoRememberMe(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)
	ttl := time.Duration(cookie.MaxAge) * time.Second

	assert.Positive(t, ttl)
	assert.Less(t, ttl, 24*time.Hour, "shorter than SPHERICON_SESSION_TTL's default")

	h.clock.t = h.clock.t.Add(ttl + time.Second)
	requireProblem(t, errOf(h.me(t, cookie.Value)), http.StatusUnauthorized)
}

func TestTheSecondStepRejectsAWrongCodeWithoutASession(t *testing.T) {
	h := newHarness(t)
	challenge := h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword).Challenge

	requireProblem(t, errOf(h.secondStep(t, challenge, "000000")), http.StatusUnauthorized)
}

func TestAReplayedCodeAndAnExpiredChallengeAreRejected(t *testing.T) {
	h := newHarness(t)
	code := h.code(t, fixtures.StaffEnrolledTotpSecret)
	first := h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword).Challenge
	_, err := h.secondStep(t, first, code)
	require.NoError(t, err)

	replay := h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword).Challenge
	requireProblem(t, errOf(h.secondStep(t, replay, code)), http.StatusUnauthorized)
	requireProblem(t, errOf(h.secondStep(t, first, h.code(t, fixtures.StaffEnrolledTotpSecret))), http.StatusUnauthorized)

	h.clock.t = h.clock.t.Add(5*time.Minute + time.Second)
	requireProblem(t, errOf(h.secondStep(t, replay, h.code(t, fixtures.StaffEnrolledTotpSecret))), http.StatusUnauthorized)
}

func TestAFirstLoginEnrolsATOTPAndOnlyTheConfirmingCodeStartsASession(t *testing.T) {
	h := newHarness(t)

	res := h.challenge(t, fixtures.StaffFreshEmail, fixtures.StaffFreshPassword)
	assert.Equal(t, staffapi.StaffLoginOutcomeEnrolment, res.Outcome)
	require.True(t, res.Enrolment.Set)
	enrolment := res.Enrolment.Value
	assert.NotEmpty(t, enrolment.Secret)
	assert.True(t, strings.HasPrefix(enrolment.URI, "otpauth://totp/"))
	assert.True(t, strings.HasPrefix(enrolment.QrCode, "data:image/png;base64,"))

	requireProblem(t, errOf(h.secondStep(t, res.Challenge, "000000")), http.StatusUnauthorized)
	again := h.challenge(t, fixtures.StaffFreshEmail, fixtures.StaffFreshPassword)
	assert.Equal(t, staffapi.StaffLoginOutcomeEnrolment, again.Outcome, "a wrong code enrols nothing")

	secret := again.Enrolment.Value.Secret
	done, err := h.secondStep(t, again.Challenge, h.code(t, secret))
	require.NoError(t, err)
	_, err = h.me(t, parseCookie(t, done.SetCookie).Value)
	require.NoError(t, err)

	h.clock.t = h.clock.t.Add(time.Minute)
	next := h.challenge(t, fixtures.StaffFreshEmail, fixtures.StaffFreshPassword)
	assert.Equal(t, staffapi.StaffLoginOutcomeChallenge, next.Outcome, "enrolled once, challenged from then on")
	assert.False(t, next.Enrolment.Set)
}

func TestLogoutClearsTheCookieAndEndsTheSessionServerSide(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)
	copied := cookie.Value

	out, err := h.env.StaffWithToken(t, cookie.Value).StaffAuthLogout(t.Context())
	require.NoError(t, err)

	cleared := parseCookie(t, out.SetCookie)
	assert.Equal(t, "STAFF_JWT", cleared.Name)
	assert.Empty(t, cleared.Value)
	assert.Negative(t, cleared.MaxAge)
	requireProblem(t, errOf(h.me(t, copied)), http.StatusUnauthorized)
}

func TestLogoutNeedsASession(t *testing.T) {
	h := newHarness(t)

	_, err := h.env.StaffAnonymous(t).StaffAuthLogout(t.Context())

	requireProblem(t, err, http.StatusUnauthorized)
}

func TestARepeatedPasswordStepKeepsTheSamePendingSecret(t *testing.T) {
	h := newHarness(t)

	first := h.challenge(t, fixtures.StaffFreshEmail, fixtures.StaffFreshPassword)
	second := h.challenge(t, fixtures.StaffFreshEmail, fixtures.StaffFreshPassword)

	require.True(t, first.Enrolment.Set)
	require.True(t, second.Enrolment.Set)
	assert.Equal(t, first.Enrolment.Value.Secret, second.Enrolment.Value.Secret, "a second password login cannot swap the secret")
	assert.Equal(t, first.Enrolment.Value.URI, second.Enrolment.Value.URI)

	secret := first.Enrolment.Value.Secret
	_, err := h.secondStep(t, first.Challenge, h.code(t, secret))
	assert.NoError(t, err, "the first challenge still confirms the secret the StaffMember enrolled")
}

func TestARepeatedPasswordStepLeavesAConfirmedStaffUntouched(t *testing.T) {
	h := newHarness(t)
	before := h.env.DB.StaffMember.GetX(t.Context(), fixtures.StaffEnrolledID)

	res := h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword)
	h.challenge(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword)

	assert.False(t, res.Enrolment.Set)
	after := h.env.DB.StaffMember.GetX(t.Context(), fixtures.StaffEnrolledID)
	assert.Equal(t, before.TotpSecretEncrypted, after.TotpSecretEncrypted)
	assert.Equal(t, before.TotpConfirmedAt, after.TotpConfirmedAt)
	assert.Equal(t, before.SessionEpoch, after.SessionEpoch)
}

func TestAUserSessionNeverOpensTheStaffSurface(t *testing.T) {
	h := newHarness(t)
	userToken := h.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	requireProblem(t, errOf(h.me(t, userToken)), http.StatusUnauthorized)
	rec := get(t, h.env.Server, "/staff/me", testhelper.RequestCookie("JWT", userToken))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAnStaffSessionNeverOpensSite(t *testing.T) {
	h := newHarness(t)
	token := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret).Value

	for _, name := range []string{"JWT", "STAFF_JWT"} {
		rec := get(t, h.env.Server, "/site/workspaces", testhelper.RequestCookie(name, token))
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "cookie %s", name)
	}
}

func TestAnStaffWhoIsAlsoACustomerKeepsTwoIdentities(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, fixtures.OwnerJohnEmail, fixtures.StaffAlsoCustomerEmail, "the fixture shares an address with a User")

	cookie := h.session(t, fixtures.StaffAlsoCustomerEmail, fixtures.StaffAlsoCustomerPassword, fixtures.StaffAlsoCustomerTotpSecret)
	_, err := h.me(t, cookie.Value)
	assert.NoError(t, err)
}

func TestEndingAStaffMembersSessionsRejectsTheOldToken(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)

	h.env.DB.StaffMember.UpdateOneID(fixtures.StaffEnrolledID).AddSessionEpoch(1).ExecX(t.Context())

	requireProblem(t, errOf(h.me(t, cookie.Value)), http.StatusUnauthorized)
}
