package condition_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func opsGroup(entries ...any) condition.Group {
	return condition.Group{Kind: "group", Combinator: "and", Rules: marshal(entries...)}
}

func TestPlainColumnOperators(t *testing.T) {
	env := testhelper.Setup(t)
	const anna, andy, bella = fixtures.OscorpAnnaEmail, fixtures.OscorpAndyEmail, fixtures.OscorpBellaEmail

	for name, tc := range map[string]struct {
		rule condition.Leaf
		want []string
	}{
		"equals":      {rule("first_name", "eq", "Anna"), []string{anna}},
		"not equals":  {rule("first_name", "neq", "Anna"), []string{andy, bella}},
		"contains":    {rule("first_name", "contains", "ELL"), []string{bella}},
		"begins with": {rule("email", "beginsWith", "an"), []string{andy, anna}},
		"null":        {rule("phone", "null", ""), []string{andy}},
		"not null":    {rule("phone", "notNull", ""), []string{anna, bella}},
	} {
		assert.Equal(t, tc.want, emailsIn(t, env, fixtures.OscorpID, opsGroup(tc.rule)), name)
	}
}

func TestCustomFieldOperators(t *testing.T) {
	env := testhelper.Setup(t)
	match := func(r condition.Leaf) []string { return emailsIn(t, env, fixtures.InitrodeID, opsGroup(r)) }

	assert.Equal(t, []string{fixtures.InitrodeOneEmail}, match(rule("custom:plan", "eq", "pro")))
	assert.Equal(t, []string{fixtures.InitrodeTwoEmail}, match(rule("custom:plan", "neq", "pro")))
	assert.Equal(t, []string{fixtures.InitrodeOneEmail, fixtures.InitrodeTwoEmail}, match(rule("custom:plan", "notNull", "")))
	// "contains" on a JSON column is membership of a JSON array, not substring.
	assert.Equal(t, []string{fixtures.InitrodeOneEmail}, match(rule("custom:tags", "contains", "beta")))
	assert.Empty(t, match(rule("custom:tags", "contains", "gamma")))
}

// The key of a custom-field rule is data, never SQL: quotes in it are matched literally
// and an injection payload is just a key nobody has.
func TestCustomFieldKeyIsABoundArgument(t *testing.T) {
	env := testhelper.Setup(t)
	match := func(r condition.Leaf) []string { return emailsIn(t, env, fixtures.InitrodeID, opsGroup(r)) }

	assert.Equal(t, []string{fixtures.InitrodeQuoteEmail}, match(rule("custom:it's", "eq", "here")))

	// Even a declared key that looks like SQL is data: the catalogue accepts any key
	// ingest created, and it only ever reaches the query as one quoted literal.
	payload := fixtures.CustomFieldInitrodeInjectionKey
	// contract-copy: four representative operators for the injection probe, not the operator list
	for _, op := range []string{"eq", "neq", "contains", "notNull"} {
		assert.Empty(t, match(rule("custom:"+payload, op, "a")), op)
	}
}

func TestCustomFieldMustBeInTheWorkspaceCatalogue(t *testing.T) {
	env := testhelper.Setup(t)
	ws := env.DB.Scoped(fixtures.AcmeID)

	for name, key := range map[string]string{
		"never declared":                 "nobody_declared_this",
		"declared by another Workspace":  fixtures.CustomFieldGlobexKey,
		"injection that nobody declared": "x' IS NOT NULL AND (SELECT count(*) FROM users)>0 AND 'a",
	} {
		def, err := json.Marshal(opsGroup(rule("custom:"+key, "eq", "a")))
		require.NoError(t, err)
		_, err = condition.ContactPredicate(context.Background(), ws, string(def))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "unknown custom field", name)
	}
}

func TestEmptyGroupsMatchEveryoneUnlessNegated(t *testing.T) {
	env := testhelper.Setup(t)

	assert.Equal(t, []string{
		fixtures.InitrodeOneEmail, fixtures.InitrodeQuoteEmail, fixtures.InitrodeThreeEmail, fixtures.InitrodeTwoEmail,
	}, emailsIn(t, env, fixtures.InitrodeID, condition.Group{Kind: "group", Combinator: "and"}))
	assert.Empty(t, emailsIn(t, env, fixtures.InitrodeID, condition.Group{Kind: "group", Combinator: "and", Not: true}),
		"an empty negated group matches no one")
}

func TestCompileRefusesWhatTheRuleSetDoesNotAllow(t *testing.T) {
	env := testhelper.Setup(t)
	for name, tc := range map[string]struct {
		rule condition.Leaf
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
		_, err := condition.Compile(context.Background(), env.DB.Scoped(fixtures.AcmeID), opsGroup(tc.rule))
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
	_, err := condition.Compile(ctx, ws, condition.Group{Kind: "group", Combinator: "and", Rules: []json.RawMessage{json.RawMessage(`42`)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed rule")

	// A nested group whose own rules are not a list.
	_, err = condition.Compile(ctx, ws, condition.Group{Kind: "group", Combinator: "and", Rules: []json.RawMessage{json.RawMessage(`{"kind":"group","combinator":"or","rules":"nope"}`)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed group")

	// A leaf whose value is not a string.
	_, err = condition.Compile(ctx, ws, condition.Group{Kind: "group", Combinator: "and", Rules: []json.RawMessage{json.RawMessage(`{"kind":"rule","field":"email","operator":"eq","value":{"a":1}}`)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed rule")

	// An entry whose kind is missing or unknown is refused, not guessed from its fields.
	_, err = condition.Compile(ctx, ws, condition.Group{Kind: "group", Combinator: "and", Rules: []json.RawMessage{json.RawMessage(`{"field":"email","operator":"eq","value":"x"}`)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown rule kind")

	// An unknown combinator is refused, not read as "and".
	_, err = condition.Compile(ctx, ws, condition.Group{Kind: "group", Combinator: "xor"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown combinator")

	// An error deep in a nested group is reported with its path.
	nested := opsGroup(opsGroup(rule("password", "eq", "x")))
	_, err = condition.Compile(ctx, ws, nested)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rule 0: rule 0: unknown field")
}

func TestContactPredicateRefusesInvalidConditions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	ws := env.DB.Scoped(fixtures.AcmeID)

	_, err := condition.ContactPredicate(ctx, ws, "{not json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid condition")

	_, err = condition.ContactPredicate(ctx, ws, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`)
	require.Error(t, err)

	p, err := condition.ContactPredicate(ctx, ws, "")
	require.NoError(t, err)
	assert.NotNil(t, p, "a blank definition matches everyone")
}
