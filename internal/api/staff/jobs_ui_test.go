package staff_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/staffmembers"
)

// The job queue's UI is served under /staff/jobs to a signed-in Staff member only: no session, a
// forged one or a customer's session cookie is refused before the UI sees the request.
func TestTheJobQueueUIServesOnlyASignedInStaffMember(t *testing.T) {
	h := newHarness(t)
	session := h.env.StaffSignIn(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword,
		func() string { return h.code(t, fixtures.StaffEnrolledTotpSecret) })

	for _, c := range []struct {
		name   string
		cookie *http.Cookie
		status int
	}{
		{"no session", nil, http.StatusUnauthorized},
		{"a forged session", &http.Cookie{Name: staffmembers.SessionCookie, Value: "forged"}, http.StatusUnauthorized},
		{"a customer's session", &http.Cookie{Name: "JWT", Value: session.Value}, http.StatusUnauthorized},
		{"a Staff member's session", session, http.StatusOK},
	} {
		rec := jobsUI(t, h.env.Server, c.cookie)
		assert.Equal(t, c.status, rec.Code, c.name)
	}
	assert.Equal(t, "jobs ui /staff/jobs/api/jobs", jobsUI(t, h.env.Server, session).Body.String(), "the UI's API is behind the guard too")
}

func jobsUI(t *testing.T, server http.Handler, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/staff/jobs/api/jobs", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}
