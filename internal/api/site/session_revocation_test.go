package site_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendWithSession sends a JSON site request carrying the session cookie (none
// when empty) and returns the recorded response.
func sendWithSession(t *testing.T, env *testhelper.TestEnv, method, path, session, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.AddCookie(testhelper.RequestCookie("JWT", session))
	}
	env.Server.ServeHTTP(rec, req)
	return rec
}

// setSession returns the JWT cookie a response sets, or nil.
func setSession(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	res := rec.Result()
	defer func() { require.NoError(t, res.Body.Close()) }()
	return sessionCookie(res)
}

// reissued returns the value of the JWT cookie a response sets, or "".
func reissued(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if c := setSession(t, rec); c != nil {
		return c.Value
	}
	return ""
}

func TestPasswordChangeEndsOtherSessionsAndKeepsTheActingOne(t *testing.T) {
	env := testhelper.Setup(t)
	other := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	acting := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	res := sendWithSession(t, env, http.MethodPut, "/site/me", acting,
		`{"currentPassword":"`+fixtures.OwnerJohnPassword+`","newPassword":"brandnewpass1"}`)
	require.Equal(t, http.StatusOK, res.Code)

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, other), "another device's session")
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, acting), "the acting cookie as it was sent")
	fresh := reissued(t, res)
	require.NotEmpty(t, fresh, "the acting session is reissued in the same response")
	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, fresh))
}

func TestRenamingKeepsEverySession(t *testing.T) {
	env := testhelper.Setup(t)
	other := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	res := sendWithSession(t, env, http.MethodPut, "/site/me", other, `{"name":"Johnny"}`)
	require.Equal(t, http.StatusOK, res.Code)

	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, other))
	assert.Empty(t, reissued(t, res), "no reissue when nothing was revoked")
}

func TestPasswordResetEndsEverySession(t *testing.T) {
	env := testhelper.Setup(t)
	old := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	res := sendWithSession(t, env, http.MethodPost, "/site/auth/forgot-password", "",
		`{"email":"`+fixtures.OwnerJohnEmail+`"}`)
	require.Equal(t, http.StatusAccepted, res.Code)
	_, body := lastSystemEmail(t, env)

	// Even when the resetting browser still carries a session, no session survives.
	res = sendWithSession(t, env, http.MethodPost, "/site/auth/reset-password", old,
		`{"token":"`+tokenFromEmail(t, body)+`","password":"brandnewpass1"}`)
	require.Equal(t, http.StatusOK, res.Code)

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, old))
	assert.Empty(t, reissued(t, res), "a reset never signs anyone in")
}

// emailChangeLink requests a change of John's login email to newEmail and returns
// the confirmation token mailed to it.
func emailChangeLink(t *testing.T, env *testhelper.TestEnv, newEmail string) string {
	t.Helper()
	res := sendWithSession(t, env, http.MethodPost, "/site/me/email-change", env.SiteToken(t, fixtures.OwnerJohnEmail, nil),
		`{"newEmail":"`+newEmail+`","currentPassword":"`+fixtures.OwnerJohnPassword+`"}`)
	require.Equal(t, http.StatusAccepted, res.Code)
	_, body := lastSystemEmail(t, env)
	return tokenFromEmail(t, body)
}

func TestEmailChangeEndsOtherSessionsAndKeepsTheConfirmingOne(t *testing.T) {
	env := testhelper.Setup(t)
	other := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	acting := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	link := emailChangeLink(t, env, "moved@example.com")

	res := sendWithSession(t, env, http.MethodPost, "/site/auth/confirm-email-change", acting, `{"token":"`+link+`"}`)
	require.Equal(t, http.StatusOK, res.Code)

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, other), "another device's session")
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, acting), "the acting cookie as it was sent")
	fresh := reissued(t, res)
	require.NotEmpty(t, fresh, "the confirming session is reissued in the same response")
	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, fresh))
}

func TestEmailChangeConfirmedWithoutASessionSignsNobodyIn(t *testing.T) {
	env := testhelper.Setup(t)
	other := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	link := emailChangeLink(t, env, "moved@example.com")

	res := sendWithSession(t, env, http.MethodPost, "/site/auth/confirm-email-change", "", `{"token":"`+link+`"}`)
	require.Equal(t, http.StatusOK, res.Code)

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, other))
	assert.Empty(t, reissued(t, res), "the link alone is not a login")
}

func TestEmailChangeConfirmedUnderAnotherUsersSessionKeepsThatSession(t *testing.T) {
	env := testhelper.Setup(t)
	link := emailChangeLink(t, env, "moved@example.com")
	someoneElse := env.SiteToken(t, fixtures.MemberMaryEmail, nil)

	res := sendWithSession(t, env, http.MethodPost, "/site/auth/confirm-email-change", someoneElse, `{"token":"`+link+`"}`)
	require.Equal(t, http.StatusOK, res.Code)

	assert.Empty(t, reissued(t, res), "John's change issues no session to another User")
	assert.Equal(t, http.StatusOK, workspacesStatus(t, env, someoneElse))
}

func TestSignOutEverywhereEndsEverySessionIncludingTheActingOne(t *testing.T) {
	env := testhelper.Setup(t)
	other := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	acting := env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	res := sendWithSession(t, env, http.MethodPost, "/site/me/sign-out-everywhere", acting, "")
	require.Equal(t, http.StatusNoContent, res.Code)

	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, other), "another device's session")
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, acting), "the acting session")
	cleared := setSession(t, res)
	require.NotNil(t, cleared, "the acting browser's cookie is cleared")
	assert.Empty(t, cleared.Value)
	assert.Negative(t, cleared.MaxAge)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionUserSignOutEverywhere)
	require.Len(t, got, 1)
	assert.Equal(t, "John", got[0].Actor.Name.Value)
	assert.Equal(t, "user", got[0].Target.Type)
}

func TestSignOutEverywhereNeedsASession(t *testing.T) {
	env := testhelper.Setup(t)
	res := sendWithSession(t, env, http.MethodPost, "/site/me/sign-out-everywhere", "", "")
	assert.Equal(t, http.StatusUnauthorized, res.Code)
}
