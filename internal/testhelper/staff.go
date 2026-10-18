package testhelper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
)

const staffBase = "http://local/staff"

// StaffAnonymous returns an /staff client carrying no session.
func (env *TestEnv) StaffAnonymous(t *testing.T) *staffapi.Client {
	t.Helper()
	return env.StaffWithToken(t, "")
}

// StaffWithToken returns an /staff client carrying the given raw session token
// in the Staff member cookie (obtained from a real two-step login, or minted for a test of
// a token in a state a login never issues).
func (env *TestEnv) StaffWithToken(t *testing.T, token string) *staffapi.Client {
	t.Helper()
	c, err := staffapi.NewClient(staffBase, staffCookieSource{token}, staffapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

type staffCookieSource struct{ token string }

func (s staffCookieSource) ApiKeyAuth(context.Context, staffapi.OperationName) (staffapi.ApiKeyAuth, error) {
	return staffapi.ApiKeyAuth{APIKey: s.token}, nil
}
