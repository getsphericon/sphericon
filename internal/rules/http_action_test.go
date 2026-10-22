package rules_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const testSigner = "dGVzdC1zZWNyZXQ="

const hookURL = "https://hooks.example.com/services/T000/B000/secret-path"

func newHTTPRule(t *testing.T, env *testhelper.TestEnv, m *rules.Module) (id int64, actionID string) {
	t.Helper()
	r, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), rules.CreateInput{
		Name:          "Call the hook",
		TriggerEvents: []string{"rule_http"},
		Actions: []rules.Action{{
			Type: rules.ActionHTTP, URL: hookURL, Body: `{"email":"{{ email }}"}`,
			Headers: []httpcall.Header{{Name: "Authorization", Value: "Bearer hunter2"}}, Secret: testSigner,
		}},
	})
	require.NoError(t, err)
	stored, err := rules.DecodeActions(r.Actions)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	return r.ID, stored[0].ID
}

func TestAnHTTPActionIsStoredSealed(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)

	id, _ := newHTTPRule(t, env, m)

	r, err := m.Get(context.Background(), env.DB.Scoped(fixtures.AcmeID), id)
	require.NoError(t, err)
	for _, secret := range []string{hookURL, "hooks.example.com", "hunter2", testSigner} {
		assert.NotContains(t, r.Actions, secret, "the stored Actions hold only ciphertext")
	}
	stored, err := rules.DecodeActions(r.Actions)
	require.NoError(t, err)
	opened, err := m.OpenHTTP(stored[0])
	require.NoError(t, err)
	assert.Equal(t, hookURL, opened.URL)
	assert.Equal(t, map[string]string{"Authorization": "Bearer hunter2"}, opened.Headers)
	assert.Equal(t, testSigner, opened.Secret)
}

func TestEditingAnHTTPActionKeepsWhatIsNotResent(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	s := env.DB.Scoped(fixtures.AcmeID)
	id, actionID := newHTTPRule(t, env, m)

	r, err := m.Update(context.Background(), s, id, rules.UpdateInput{Actions: &[]rules.Action{{
		Type: rules.ActionHTTP, ID: actionID, Body: `{"changed":true}`,
		Headers: []httpcall.Header{{Name: "Authorization"}},
	}}})

	require.NoError(t, err)
	stored, err := rules.DecodeActions(r.Actions)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, actionID, stored[0].ID)
	opened, err := m.OpenHTTP(stored[0])
	require.NoError(t, err)
	assert.Equal(t, hookURL, opened.URL)
	assert.Equal(t, "Bearer hunter2", opened.Headers["Authorization"])
	assert.Equal(t, testSigner, opened.Secret)
	assert.JSONEq(t, `{"changed":true}`, stored[0].Body)
}

func TestAnInvalidHTTPActionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	cases := map[string]rules.Action{
		"no url":                  {Type: rules.ActionHTTP, Body: `{}`},
		"not http":                {Type: rules.ActionHTTP, URL: "ftp://example.com", Body: `{}`},
		"a body that is not JSON": {Type: rules.ActionHTTP, URL: hookURL, Body: `not json`},
		"a reserved header":       {Type: rules.ActionHTTP, URL: hookURL, Body: `{}`, Headers: []httpcall.Header{{Name: "Host", Value: "x"}}},
	}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), rules.CreateInput{
				Name: "Bad", TriggerEvents: []string{"rule_http"}, Actions: []rules.Action{a},
			})
			assert.ErrorIs(t, err, rules.ErrInvalid)
		})
	}
}
