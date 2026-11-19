package rules_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/httpheader"
	"github.com/getsphericon/sphericon/internal/kernel/ruleaction"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const testSigner = "dGVzdC1zZWNyZXQ="

const hookURL = "https://hooks.example.com/services/T000/B000/secret-path"

func newHTTPRule(t *testing.T, env *testhelper.TestEnv, m *rules.Module) (id int64, actionID string) {
	t.Helper()
	r, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), rules.CreateInput{
		Name:          "Call the hook",
		TriggerEvents: []string{"rule_http"},
		Actions: []ruleaction.Action{{
			Type: ruleaction.HTTP, URL: hookURL, Body: `{"email":"{{ email }}"}`,
			Headers: []httpheader.Header{{Name: "Authorization", Value: "Bearer hunter2"}}, Secret: testSigner,
		}},
	})
	require.NoError(t, err)
	stored := draftOf(t, r).Actions
	require.Len(t, stored, 1)
	return r.ID, stored[0].ID
}

func TestAnHTTPActionIsStoredSealed(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus, nil, time.Now)

	id, _ := newHTTPRule(t, env, m)

	r, err := m.Get(context.Background(), env.DB.Scoped(fixtures.AcmeID), id)
	require.NoError(t, err)
	for _, secret := range []string{hookURL, "hooks.example.com", "hunter2", testSigner} {
		assert.NotContains(t, testhelper.Document(t, draftOf(t, r).Actions), secret, "the stored Actions hold only ciphertext")
	}
	stored := draftOf(t, r).Actions
	opened, err := m.OpenHTTP(stored[0])
	require.NoError(t, err)
	assert.Equal(t, hookURL, opened.URL)
	assert.Equal(t, map[string]string{"Authorization": "Bearer hunter2"}, opened.Headers)
	assert.Equal(t, testSigner, opened.Secret)
}

// draftOf is the Rule's draft, which a test that just wrote it expects.
func draftOf(t *testing.T, r *ent.Rule) *ent.RuleVersion {
	t.Helper()
	_, draft := rules.LiveAndDraft(r)
	require.NotNil(t, draft)
	return draft
}

// The edit is a new draft above the published version, and the http Action keeps what the live
// version holds.
func TestEditingAnHTTPActionKeepsWhatIsNotResent(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus, nil, time.Now)
	s := env.DB.Scoped(fixtures.AcmeID)
	id, actionID := newHTTPRule(t, env, m)
	_, err := m.Publish(context.Background(), s, id, testhelper.RuleRevisionIn(t.Context(), t, s, id))
	require.NoError(t, err)

	r, err := m.Update(context.Background(), s, id, rules.UpdateInput{Revision: testhelper.RuleRevisionIn(t.Context(), t, s, id), Actions: &[]ruleaction.Action{{
		Type: ruleaction.HTTP, ID: actionID, Body: `{"changed":true}`,
		Headers: []httpheader.Header{{Name: "Authorization"}},
	}}})

	require.NoError(t, err)
	stored := draftOf(t, r).Actions
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
	m := rules.New(env.Cipher, env.Bus, nil, time.Now)
	cases := map[string]ruleaction.Action{
		"no url":                  {Type: ruleaction.HTTP, Body: `{}`},
		"not http":                {Type: ruleaction.HTTP, URL: "ftp://example.com", Body: `{}`},
		"a body that is not JSON": {Type: ruleaction.HTTP, URL: hookURL, Body: `not json`},
		"a reserved header":       {Type: ruleaction.HTTP, URL: hookURL, Body: `{}`, Headers: []httpheader.Header{{Name: "Host", Value: "x"}}},
		"an empty secret":         {Type: ruleaction.HTTP, URL: hookURL, Body: `{}`, Secret: "whsec_"},
	}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), rules.CreateInput{
				Name: "Bad", TriggerEvents: []string{"rule_http"}, Actions: []ruleaction.Action{a},
			})
			assert.ErrorIs(t, err, rules.ErrInvalid)
		})
	}
}
