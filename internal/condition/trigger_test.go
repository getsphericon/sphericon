package condition_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// triggerMatches reports whether g, compiled as a Rule's Condition against props, holds for the
// Contact.
func triggerMatches(t require.TestingT, s *ent.Scoped, g condition.Group, props map[string]any, id int64) bool {
	ctx := context.Background()
	p, err := condition.RulePredicate(ctx, s, encode(t, g), props)
	require.NoError(t, err)
	ok, err := s.Contact().Query().Where(contact.ID(id), p).Exist(ctx)
	require.NoError(t, err)
	return ok
}

// One operator table serves a Contact attribute (SQL) and an Event property (Go): for any
// attribute, operator, operand and Contact, the two sides agree.
func TestOperatorsAgreeOnTheContactSideAndTheEventSide(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.OscorpID)
	contacts, err := s.Contact().Query().All(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, contacts)
	attribute := map[string]func(*ent.Contact) *string{
		"first_name": func(c *ent.Contact) *string { return c.FirstName },
		"phone":      func(c *ent.Contact) *string { return c.Phone },
	}

	rapid.Check(t, func(rt *rapid.T) {
		field := rapid.SampledFrom([]string{"first_name", "phone"}).Draw(rt, "field")
		// contract-copy: the operator names of the grammar, not a list this test owns
		op := rapid.SampledFrom([]string{"eq", "neq", "contains", "beginsWith", "null", "notNull"}).Draw(rt, "operator")
		want := rapid.SampledFrom([]string{"Anna", "anna", "ELL", "an", "", "zzz"}).Draw(rt, "operand")
		c := rapid.SampledFrom(contacts).Draw(rt, "contact")
		have := property(attribute[field](c))

		viaSQL := triggerMatches(rt, s, opsGroup(rule(field, op, want)), nil, c.ID)
		viaGo := triggerMatches(rt, s, opsGroup(rule("trigger:k", op, want)), map[string]any{"k": have}, c.ID)

		assert.Equal(rt, viaSQL, viaGo, "%s %s %q on %v", field, op, want, have)
	})
}

func TestTriggerLeafHoldsForAStringOfThatKey(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)

	assert.True(t, triggerMatches(t, s, opsGroup(rule("trigger:amount", "eq", "10")), map[string]any{"amount": "10"}, fixtures.ContactAliceID))
}

// A property that is missing, or not a string, satisfies no operator that needs one.
func TestTriggerLeafIsFalseForAMissingKeyOrAnotherType(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	notStrings := []map[string]any{
		{"other": "10"}, nil, {"amount": 10}, {"amount": true},
		{"amount": map[string]any{"v": "10"}}, {"amount": []any{"10"}}, {"amount": nil},
	}

	rapid.Check(t, func(rt *rapid.T) {
		// contract-copy: operators that need a string; null is tested separately
		op := rapid.SampledFrom([]string{"eq", "neq", "contains", "beginsWith", "notNull"}).Draw(rt, "operator")
		props := rapid.SampledFrom(notStrings).Draw(rt, "props")

		holds := triggerMatches(rt, s, opsGroup(rule("trigger:amount", op, "10")), props, fixtures.ContactAliceID)

		assert.False(rt, holds, "%s on %v", op, props)
	})
}

func TestTriggerLeafNullMeansAnExplicitNullNotAMissingKey(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	null := opsGroup(rule("trigger:amount", "null", ""))

	assert.True(t, triggerMatches(t, s, null, map[string]any{"amount": nil}, fixtures.ContactAliceID), "an explicit null is empty")
	assert.False(t, triggerMatches(t, s, null, map[string]any{}, fixtures.ContactAliceID), "a missing key is false, not empty")
}

func TestTriggerLeafFoldsIntoTheRestOfTheTree(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.OscorpID)
	anna := env.DB.Contact.Query().Where(contact.Email(fixtures.OscorpAnnaEmail)).OnlyX(context.Background()).ID
	props := map[string]any{"url": "https://x.test/pricing"}
	hit := rule("trigger:url", "contains", "pricing")
	miss := rule("trigger:url", "contains", "docs")
	annaRow := rule("first_name", "eq", "Anna")

	assert.True(t, triggerMatches(t, s, opsGroup(hit, annaRow), props, anna))
	assert.False(t, triggerMatches(t, s, opsGroup(miss, annaRow), props, anna))
	or := condition.Group{Kind: "group", Combinator: "or", Rules: marshal(miss, annaRow)}
	assert.True(t, triggerMatches(t, s, or, props, anna))
	not := condition.Group{Kind: "group", Combinator: "and", Not: true, Rules: marshal(miss)}
	assert.True(t, triggerMatches(t, s, not, props, anna), "not of a false leaf holds")
	assert.True(t, triggerMatches(t, s, opsGroup(opsGroup(hit)), props, anna), "inside a nested group")
}

func TestTriggerLeafIsValidOnlyInARule(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	g := opsGroup(rule("trigger:amount", "eq", "10"))

	_, err := condition.Compile(ctx, s, g)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only in a Rule")
	require.Error(t, validate(t, env, encode(t, g)))

	// Any key is valid on save: nothing says which properties an Event will carry.
	_, err = condition.RulePredicate(ctx, s, encode(t, g), nil)
	require.NoError(t, err)

	for name, bad := range map[string]condition.Leaf{
		"no key":           rule("trigger:", "eq", "x"),
		"unknown operator": rule("trigger:k", "performed", "x"),
	} {
		_, err = condition.RulePredicate(ctx, s, encode(t, opsGroup(bad)), nil)
		require.Error(t, err, name)
	}
}

// property is an attribute as an Event property: its text, or JSON null when the attribute is null.
func property(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func encode(t require.TestingT, g condition.Group) string {
	b, err := json.Marshal(g)
	require.NoError(t, err)
	return string(b)
}
