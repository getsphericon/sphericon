package segments_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// opsEmails returns the emails of this test's contacts (suffix @ops.test) matching g.
func opsEmails(t *testing.T, env *testhelper.TestEnv, g segments.Group) []string {
	t.Helper()
	ctx := context.Background()
	p, err := segments.Compile(ctx, env.DB.Scoped(fixtures.AcmeID), g)
	require.NoError(t, err)
	got, err := env.DB.Contact.Query().
		Where(contact.WorkspaceID(fixtures.AcmeID), contact.EmailContainsFold("@ops.test"), p).
		Order(contact.ByEmail()).
		All(ctx)
	require.NoError(t, err)
	out := make([]string, len(got))
	for i, c := range got {
		out[i] = *c.Email
	}
	return out
}

// declare adds keys to the Acme Custom field catalogue (a rule may only name a declared key).
func declare(t *testing.T, env *testhelper.TestEnv, keys ...string) {
	t.Helper()
	for _, key := range keys {
		require.NoError(t, env.DB.Scoped(fixtures.AcmeID).CustomField().Create().SetKey(key).SetName(key).Exec(context.Background()))
	}
}

func opsGroup(entries ...any) segments.Group {
	return segments.Group{Kind: "group", Combinator: "and", Rules: marshal(entries...)}
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
		"equals":      {rule("first_name", "eq", "Anna"), []string{"anna@ops.test"}},
		"not equals":  {rule("first_name", "neq", "Anna"), []string{"andy@ops.test", "bella@ops.test"}},
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
	declare(t, env, "tags")
	for email, custom := range map[string]map[string]any{
		"one@ops.test":   {"plan": "pro", "tags": []any{"alpha", "beta"}},
		"two@ops.test":   {"plan": "free"},
		"three@ops.test": {"other": 1},
	} {
		_, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail(email).SetCustomFields(custom).Save(ctx)
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"one@ops.test"}, opsEmails(t, env, opsGroup(rule("custom:plan", "eq", "pro"))))
	assert.Equal(t, []string{"two@ops.test"}, opsEmails(t, env, opsGroup(rule("custom:plan", "neq", "pro"))))
	assert.Equal(t, []string{"one@ops.test", "two@ops.test"}, opsEmails(t, env, opsGroup(rule("custom:plan", "notNull", ""))))
	// "contains" on a JSON column is membership of a JSON array, not substring.
	assert.Equal(t, []string{"one@ops.test"}, opsEmails(t, env, opsGroup(rule("custom:tags", "contains", "beta"))))
	assert.Empty(t, opsEmails(t, env, opsGroup(rule("custom:tags", "contains", "gamma"))))
}

// The key of a custom-field rule is data, never SQL: quotes in it are matched literally
// and an injection payload is just a key nobody has.
func TestCustomFieldKeyIsABoundArgument(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	declare(t, env, "it's")
	_, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("quote@ops.test").
		SetCustomFields(map[string]any{"it's": "here"}).Save(ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"quote@ops.test"}, opsEmails(t, env, opsGroup(rule("custom:it's", "eq", "here"))))

	// Even a declared key that looks like SQL is data: the catalogue accepts any key
	// ingest created, and it only ever reaches the query as a bound argument.
	payload := "x' IS NOT NULL AND (SELECT count(*) FROM users)>0 AND 'a"
	declare(t, env, payload)
	for _, op := range []string{"eq", "neq", "contains", "notNull"} {
		assert.Empty(t, opsEmails(t, env, opsGroup(rule("custom:"+payload, op, "a"))), op)
	}
}

func TestCustomFieldMustBeInTheWorkspaceCatalogue(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New()
	ws := env.DB.Scoped(fixtures.AcmeID)

	for name, key := range map[string]string{
		"never declared":                 "nobody_declared_this",
		"declared by another Workspace":  fixtures.CustomFieldGlobexKey,
		"injection that nobody declared": "x' IS NOT NULL AND (SELECT count(*) FROM users)>0 AND 'a",
	} {
		def, err := json.Marshal(opsGroup(rule("custom:"+key, "eq", "a")))
		require.NoError(t, err)
		_, err = m.Preview(context.Background(), ws, string(def))
		require.ErrorIs(t, err, segments.ErrInvalidDefinition, name)
		assert.Contains(t, err.Error(), "unknown custom field", name)
	}
}

func TestEmptyGroupsMatchEveryoneUnlessNegated(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("only@ops.test").Save(ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"only@ops.test"}, opsEmails(t, env, segments.Group{Kind: "group", Combinator: "and"}))
	assert.Empty(t, opsEmails(t, env, segments.Group{Kind: "group", Combinator: "and", Not: true}), "an empty negated group matches no one")
}

func TestCompileRefusesWhatTheRuleSetDoesNotAllow(t *testing.T) {
	env := testhelper.Setup(t)
	for name, tc := range map[string]struct {
		rule segments.Rule
		msg  string
	}{
		"unknown column":                {rule("password", "eq", "x"), "unknown field"},
		"unsupported plain operator":    {rule("email", "regex", "x"), "unsupported operator"},
		"event without action":          {rule("event:", "performed", ""), "unknown field"},
		"unsupported event operator":    {rule("event:signup", "eq", ""), "for event field"},
		"bad event window":              {rule("event:signup", "performed", "abc"), "event window"},
		"negative event window":         {rule("event:signup", "performed", "-3"), "event window"},
		"unsupported tag operator":      {rule("tag", "eq", "vip"), "for tag field"},
		"blank tag name":                {rule("tag", "has", "   "), "tag name is required"},
		"unsupported json operator":     {rule("custom:plan", "beginsWith", "p"), "for json field"},
		"json prefix without a key":     {rule("custom:", "eq", "x"), "unknown field"},
		"plain operator on json column": {rule("custom:plan", "null", ""), "for json field"},
	} {
		_, err := segments.Compile(context.Background(), env.DB.Scoped(fixtures.AcmeID), opsGroup(tc.rule))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.msg, name)
		assert.Contains(t, err.Error(), "rule 0", "%s: the error names the offending rule", name)
	}
}

func TestCompileRefusesMalformedEntries(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	ws := env.DB.Scoped(fixtures.AcmeID)

	// A rules[] entry that is not an object at all.
	_, err := segments.Compile(ctx, ws, segments.Group{Kind: "group", Combinator: "and", Rules: []json.RawMessage{json.RawMessage(`42`)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed rule")

	// A nested group whose own rules are not a list.
	_, err = segments.Compile(ctx, ws, segments.Group{Kind: "group", Combinator: "and", Rules: []json.RawMessage{json.RawMessage(`{"kind":"group","combinator":"or","rules":"nope"}`)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed group")

	// A leaf whose value is not a string.
	_, err = segments.Compile(ctx, ws, segments.Group{Kind: "group", Combinator: "and", Rules: []json.RawMessage{json.RawMessage(`{"kind":"rule","field":"email","operator":"eq","value":{"a":1}}`)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed rule")

	// An entry whose kind is missing or unknown is refused, not guessed from its fields.
	_, err = segments.Compile(ctx, ws, segments.Group{Kind: "group", Combinator: "and", Rules: []json.RawMessage{json.RawMessage(`{"field":"email","operator":"eq","value":"x"}`)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown rule kind")

	// An unknown combinator is refused, not read as "and".
	_, err = segments.Compile(ctx, ws, segments.Group{Kind: "group", Combinator: "xor"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown combinator")

	// An error deep in a nested group is reported with its path.
	nested := opsGroup(opsGroup(rule("password", "eq", "x")))
	_, err = segments.Compile(ctx, ws, nested)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rule 0: rule 0: unknown field")
}

func TestContactPredicateRefusesInvalidDefinitions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	ws := env.DB.Scoped(fixtures.AcmeID)

	_, err := segments.ContactPredicate(ctx, ws, "{not json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid segment definition")

	_, err = segments.ContactPredicate(ctx, ws, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`)
	require.Error(t, err)

	p, err := segments.ContactPredicate(ctx, ws, "")
	require.NoError(t, err)
	assert.NotNil(t, p, "a blank definition matches everyone")
}
