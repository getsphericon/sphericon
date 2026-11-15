package workspaceoversight_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/testhelper"
)

type harness struct {
	env   *testhelper.TestEnv
	clock *testhelper.Clock
}

func newHarness(t *testing.T, opts ...testhelper.Option) *harness {
	t.Helper()
	c := testhelper.NewClock(time.Now())
	return &harness{env: testhelper.Setup(t, append([]testhelper.Option{testhelper.WithClock(c.Now)}, opts...)...), clock: c}
}

// code is the authenticator code of secret at the env clock.
func (h *harness) code(t *testing.T, secret string) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, h.clock.Now())
	require.NoError(t, err)
	return c
}

// session runs the whole two-step login of an enrolled Staff member on the core surface and
// returns the session cookie.
func (h *harness) session(t *testing.T, email, password, secret string) *http.Cookie {
	t.Helper()
	return h.env.StaffSignIn(t, email, password, func() string { return h.code(t, secret) })
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
