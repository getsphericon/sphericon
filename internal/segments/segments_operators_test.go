package segments_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// opsEmails returns the emails of this test's contacts (suffix @ops.test) matching g.
func opsEmails(t *testing.T, env *testhelper.TestEnv, g segments.Group) []string {
	t.Helper()
	p, err := segments.Compile(g, segments.ContactSchema())
	require.NoError(t, err)
	got, err := env.DB.Contact.Query().
		Where(contact.WorkspaceID(fixtures.AcmeID), contact.EmailContainsFold("@ops.test"), predicate.Contact(p)).
		Order(contact.ByEmail()).
		All(context.Background())
	require.NoError(t, err)
	out := make([]string, len(got))
	for i, c := range got {
		out[i] = *c.Email
	}
	return out
}

func opsGroup(entries ...any) segments.Group {
	return segments.Group{Combinator: "and", Rules: marshal(entries...)}
}

func TestPlainColumnOperators(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	for _, c := range []struct{ email, first, phone string }{
		{"anna@ops.test", "Anna", "+1555"},
		{"andy@ops.test", "Andy", ""},
		{"bella@ops.test", "Bella", "+1666"},
	} {
		q := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail(c.email).SetFirstName(c.first)
		if c.phone != "" {
			q.SetPhone(c.phone)
		}
		_, err := q.Save(ctx)
		require.NoError(t, err)
	}

	for name, tc := range map[string]struct {
		rule segments.Rule
		want []string
	}{
		"equals":      {rule("first_name", "=", "Anna"), []string{"anna@ops.test"}},
		"not equals":  {rule("first_name", "!=", "Anna"), []string{"andy@ops.test", "bella@ops.test"}},
		"contains":    {rule("first_name", "contains", "ELL"), []string{"bella@ops.test"}},
		"begins with": {rule("email", "beginsWith", "an"), []string{"andy@ops.test", "anna@ops.test"}},
		"null":        {rule("phone", "null", ""), []string{"andy@ops.test"}},
		"not null":    {rule("phone", "notNull", ""), []string{"anna@ops.test", "bella@ops.test"}},
	} {
		assert.Equal(t, tc.want, opsEmails(t, env, opsGroup(tc.rule)), name)
	}
}

func TestCustomFieldOperators(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	for email, custom := range map[string]map[string]any{
		"one@ops.test":   {"plan": "pro", "tags": []any{"alpha", "beta"}},
		"two@ops.test":   {"plan": "free"},
		"three@ops.test": {"other": 1},
	} {
		_, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail(email).SetCustomFields(custom).Save(ctx)
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"one@ops.test"}, opsEmails(t, env, opsGroup(rule("custom:plan", "=", "pro"))))
	assert.Equal(t, []string{"two@ops.test"}, opsEmails(t, env, opsGroup(rule("custom:plan", "!=", "pro"))))
	assert.Equal(t, []string{"one@ops.test", "two@ops.test"}, opsEmails(t, env, opsGroup(rule("custom:plan", "notNull", ""))))
	// "contains" on a JSON column is membership of a JSON array, not substring.
	assert.Equal(t, []string{"one@ops.test"}, opsEmails(t, env, opsGroup(rule("custom:tags", "contains", "beta"))))
	assert.Empty(t, opsEmails(t, env, opsGroup(rule("custom:tags", "contains", "gamma"))))
}

func TestEmptyGroupsMatchEveryoneUnlessNegated(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("only@ops.test").Save(ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"only@ops.test"}, opsEmails(t, env, segments.Group{Combinator: "and"}))
	assert.Empty(t, opsEmails(t, env, segments.Group{Combinator: "and", Not: true}), "an empty negated group matches no one")
}

func TestCompileRefusesWhatTheSchemaDoesNotAllow(t *testing.T) {
	schema := segments.ContactSchema()
	bare := segments.Schema{Columns: map[string]string{"email": "email"}}
	for name, tc := range map[string]struct {
		schema segments.Schema
		rule   segments.Rule
		msg    string
	}{
		"unknown column":                {schema, rule("password", "=", "x"), "unknown field"},
		"unsupported plain operator":    {schema, rule("email", "regex", "x"), "unsupported operator"},
		"event without action":          {schema, rule("event:", "performed", ""), "unknown field"},
		"event field without schema":    {bare, rule("event:signup", "performed", ""), "unknown field"},
		"unsupported event operator":    {schema, rule("event:signup", "=", ""), "for event field"},
		"bad event window":              {schema, rule("event:signup", "performed", "abc"), "event window"},
		"negative event window":         {schema, rule("event:signup", "performed", "-3"), "event window"},
		"unsupported tag operator":      {schema, rule("tag", "=", "vip"), "for tag field"},
		"blank tag name":                {schema, rule("tag", "has", "   "), "tag name is required"},
		"unsupported json operator":     {schema, rule("custom:plan", "beginsWith", "p"), "for json field"},
		"json prefix without a path":    {schema, rule("custom:", "=", "x"), "unknown field"},
		"tag field without tag schema":  {bare, rule("tag", "has", "vip"), "unknown field"},
		"plain operator on json column": {schema, rule("custom:plan", "null", ""), "for json field"},
	} {
		_, err := segments.Compile(opsGroup(tc.rule), tc.schema)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.msg, name)
		assert.Contains(t, err.Error(), "rule 0", "%s: the error names the offending rule", name)
	}
}

func TestCompileRefusesMalformedEntries(t *testing.T) {
	schema := segments.ContactSchema()

	// A rules[] entry that is not an object at all.
	_, err := segments.Compile(segments.Group{Rules: []json.RawMessage{json.RawMessage(`42`)}}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed rule")

	// A nested group whose own rules are not a list.
	_, err = segments.Compile(segments.Group{Rules: []json.RawMessage{json.RawMessage(`{"combinator":"or","rules":"nope"}`)}}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed group")

	// A leaf whose value is not a string.
	_, err = segments.Compile(segments.Group{Rules: []json.RawMessage{json.RawMessage(`{"field":"email","operator":"=","value":{"a":1}}`)}}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed rule")

	// An error deep in a nested group is reported with its path.
	nested := opsGroup(opsGroup(rule("password", "=", "x")))
	_, err = segments.Compile(nested, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rule 0: rule 0: unknown field")
}

func TestContactPredicateRefusesInvalidDefinitions(t *testing.T) {
	_, err := segments.ContactPredicate("{not json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid segment definition")

	_, err = segments.ContactPredicate(`{"combinator":"and","rules":[{"field":"nope","operator":"=","value":"x"}]}`)
	require.Error(t, err)

	p, err := segments.ContactPredicate("")
	require.NoError(t, err)
	assert.NotNil(t, p, "a blank definition matches everyone")
}
