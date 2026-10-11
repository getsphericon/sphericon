package testhelper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	operatorapi "github.com/mokevnin/sphericon/gen/operator"
)

const operatorBase = "http://local/operator"

// OperatorAnonymous returns an /operator client carrying no session.
func (env *TestEnv) OperatorAnonymous(t *testing.T) *operatorapi.Client {
	t.Helper()
	return env.OperatorWithToken(t, "")
}

// OperatorWithToken returns an /operator client carrying the given raw session token
// in the Operator cookie (obtained from a real two-step login, or minted for a test of
// a token in a state a login never issues).
func (env *TestEnv) OperatorWithToken(t *testing.T, token string) *operatorapi.Client {
	t.Helper()
	c, err := operatorapi.NewClient(operatorBase, operatorCookieSource{token}, operatorapi.WithClient(env.Transport(nil)))
	require.NoError(t, err)
	return c
}

type operatorCookieSource struct{ token string }

func (s operatorCookieSource) ApiKeyAuth(context.Context, operatorapi.OperationName) (operatorapi.ApiKeyAuth, error) {
	return operatorapi.ApiKeyAuth{APIKey: s.token}, nil
}
