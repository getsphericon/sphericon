package condition_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// triggerMatches reports whether g, compiled as a Rule's Condition against props, holds for the
// Contact.
func triggerMatches(t *testing.T, s *ent.Scoped, g condition.Group, props map[string]any, id int64) bool {
	t.Helper()
	ctx := context.Background()
	p, err := condition.RulePredicate(ctx, s, encode(t, g), props)
	require.NoError(t, err)
	ok, err := s.Contact().Query().Where(contact.ID(id), p).Exist(ctx)
	require.NoError(t, err)
	return ok
}

// One operator table serves a Contact attribute (SQL) and an Event property (Go): the same
// cases, run through each, agree for every Contact and every operator.
func TestOperatorsAgreeOnTheContactSideAndTheEventSide(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.OscorpID)
	contacts, err := s.Contact().Query().All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, contacts)

	attribute := map[string]func(*ent.Contact) *string{
		"first_name": func(c *ent.Contact) *string { return c.FirstName },
		"phone":      func(c *ent.Contact) *string { return c.Phone },
	}
	wants := []string{"Anna", "anna", "ELL", "an", "", "zzz"}
	// contract-copy: the operator names of the grammar, not a list this test owns
	for _, op := range []string{"eq", "neq", "contains", "beginsWith", "null", "notNull"} {
		for field, get := range attribute {
			for _, want := range wants {
				for _, c := range contacts {
					have := property(get(c))
					viaSQL := triggerMatches(t, s, opsGroup(rule(field, op, want)), nil, c.ID)
					viaGo := triggerMatches(t, s, opsGroup(rule("trigger:k", op, want)), map[string]any{"k": have}, c.ID)
					assert.Equal(t, viaSQL, viaGo, "%s %s %q on %v", field, op, want, have)
				}
			}
		}
	}
}

func TestTriggerLeafIsFalseForAMissingKeyOrAnotherType(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	var id int64 = fixtures.ContactAliceID
	holds := func(op, want string, props map[string]any) bool {
		return triggerMatches(t, s, opsGroup(rule("trigger:amount", op, want)), props, id)
	}

	assert.True(t, holds("eq", "10", map[string]any{"amount": "10"}))
	for name, props := range map[string]map[string]any{
		"missing key":  {"other": "10"},
		"no props":     nil,
		"number":       {"amount": 10},
		"bool":         {"amount": true},
		"object":       {"amount": map[string]any{"v": "10"}},
		"list":         {"amount": []any{"10"}},
		"explicit nil": {"amount": nil},
	} {
		// contract-copy: operators that need a string; null is tested separately
		for _, op := range []string{"eq", "neq", "contains", "beginsWith", "notNull"} {
			assert.False(t, holds(op, "10", props), "%s %s", name, op)
		}
	}
	assert.True(t, holds("null", "", map[string]any{"amount": nil}), "an explicit null is empty")
	assert.False(t, holds("null", "", map[string]any{}), "a missing key is false, not empty")
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

func encode(t *testing.T, g condition.Group) string {
	t.Helper()
	b, err := json.Marshal(g)
	require.NoError(t, err)
	return string(b)
}
