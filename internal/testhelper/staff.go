package testhelper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
)

const staffBase = "http://local/staff"

// StaffAnonymous returns a core /staff client carrying no session.
func (env *TestEnv) StaffAnonymous(t *testing.T) *staffapi.Client {
	t.Helper()
	return env.StaffWithToken(t, "")
}

// StaffWithToken returns a core /staff client carrying the given raw session token
// in the Staff member cookie (obtained from a real two-step login, or minted for a test of
// a token in a state a login never issues).
func (env *TestEnv) StaffWithToken(t *testing.T, token string) *staffapi.Client {
	t.Helper()
	c, err := staffapi.NewClient(staffBase, staffCookieSource{token}, staffapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

// StaffEnterpriseWithToken returns a client of the Staff console's Enterprise sections
// carrying the given raw session token in the Staff member cookie.
func (env *TestEnv) StaffEnterpriseWithToken(t *testing.T, token string) *staffeeapi.Client {
	t.Helper()
	c, err := staffeeapi.NewClient(staffBase, staffEnterpriseCookieSource{token}, staffeeapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

// StaffSignIn runs the two-step sign-in of a Staff member with its password and a current
// TOTP code and returns the session cookie it sets.
func (env *TestEnv) StaffSignIn(t *testing.T, email, password string, code func() string) *http.Cookie {
	t.Helper()
	c := env.StaffAnonymous(t)
	res, err := c.StaffAuthLogin(t.Context(), &staffapi.StaffLoginInput{Email: email, Password: password})
	require.NoError(t, err)
	challenge, ok := res.(*staffapi.StaffLoginResult)
	require.Truef(t, ok, "the password step grants a challenge, got %T", res)
	done, err := c.StaffAuthSecondFactor(t.Context(), &staffapi.StaffSecondFactorInput{Challenge: challenge.Challenge, Code: code()})
	require.NoError(t, err)
	session, ok := done.(*staffapi.StaffResourceHeaders)
	require.Truef(t, ok, "the second step starts a session, got %T", done)
	rec := httptest.NewRecorder()
	rec.Header().Add("Set-Cookie", session.SetCookie)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

type staffCookieSource struct{ token string }

func (s staffCookieSource) ApiKeyAuth(context.Context, staffapi.OperationName) (staffapi.ApiKeyAuth, error) {
	return staffapi.ApiKeyAuth{APIKey: s.token}, nil
}

type staffEnterpriseCookieSource struct{ token string }

func (s staffEnterpriseCookieSource) ApiKeyAuth(context.Context, staffeeapi.OperationName) (staffeeapi.ApiKeyAuth, error) {
	return staffeeapi.ApiKeyAuth{APIKey: s.token}, nil
}

// JobsUIStub stands in for the job queue's UI (River UI) in tests: it answers 200 with the path it
// was asked for, so a test sees which requests the Staff session guard let through.
var JobsUIStub = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("jobs ui " + r.URL.Path))
})
