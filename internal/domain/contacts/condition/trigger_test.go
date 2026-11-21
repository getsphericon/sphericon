package condition_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// triggerMatches reports whether g, compiled as a Rule's Condition against props, holds for the
// Contact.
func triggerMatches(t require.TestingT, s *ent.Scoped, g conditiontree.Rule, props map[string]any, id int64) bool {
	ctx := context.Background()
	p, err := condition.New(time.Now).RulePredicate(ctx, s, g, props)
	require.NoError(t, err)
	ok, err := s.Contact().Query().Where(contact.ID(id), p).Exist(ctx)
	require.NoError(t, err)
	return ok
}

// One table of the text and presence tests serves a Contact attribute (SQL) and an Event property
// (Go): for any attribute, test, operand and Contact, the two sides agree.
func TestOperatorsAgreeOnTheContactSideAndTheEventSide(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.OscorpID)
	contacts, err := s.Contact().Query().All(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, contacts)
	attributes := map[string]func(*ent.Contact) *string{
		"first_name": func(c *ent.Contact) *string { return c.FirstName },
		"phone":      func(c *ent.Contact) *string { return c.Phone },
	}

	rapid.Check(t, func(rt *rapid.T) {
		field := rapid.SampledFrom([]string{"first_name", "phone"}).Draw(rt, "field")
		want := rapid.SampledFrom([]string{"Anna", "anna", "ELL", "an", "", "zzz"}).Draw(rt, "operand")
		texts := lo.Map(siteapi.TextOperator("").AllValues(), func(op siteapi.TextOperator, _ int) leaf { return text(string(op), want) })
		presences := lo.Map(siteapi.PresenceOperator("").AllValues(), func(op siteapi.PresenceOperator, _ int) leaf { return presence(string(op)) })
		test := rapid.SampledFrom(append(texts, presences...)).Draw(rt, "test")
		c := rapid.SampledFrom(contacts).Draw(rt, "contact")
		have := asProperty(attributes[field](c))

		viaSQL := triggerMatches(rt, s, ruleOf(attribute(field, test)), nil, c.ID)
		viaGo := triggerMatches(rt, s, ruleOf(property("k", test)), map[string]any{"k": have}, c.ID)

		assert.Equal(rt, viaSQL, viaGo, "%s %v on %v", field, test, have)
	})
}

func TestPropertyTestHoldsForAStringOfThatKey(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)

	holds := triggerMatches(t, s, ruleOf(property("amount", text("eq", "10"))), map[string]any{"amount": "10"}, fixtures.ContactAliceID)

	assert.True(t, holds)
}

// A property that is missing, or not a string, passes no test that needs one.
func TestPropertyTestIsFalseForAMissingKeyOrAnotherType(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	notStrings := []map[string]any{
		{"other": "10"}, nil, {"amount": 10}, {"amount": true},
		{"amount": map[string]any{"v": "10"}}, {"amount": []any{"10"}}, {"amount": nil},
	}

	rapid.Check(t, func(rt *rapid.T) {
		// The tests that need a string; notSet is tested separately.
		texts := lo.Map(siteapi.TextOperator("").AllValues(), func(op siteapi.TextOperator, _ int) leaf { return text(string(op), "10") })
		test := rapid.SampledFrom(append(texts, presence("set"))).Draw(rt, "test")
		props := rapid.SampledFrom(notStrings).Draw(rt, "props")

		holds := triggerMatches(rt, s, ruleOf(property("amount", test)), props, fixtures.ContactAliceID)

		assert.False(rt, holds, "%v on %v", test, props)
	})
}

func TestPropertyNotSetMeansAnExplicitNullNotAMissingKey(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	notSet := ruleOf(property("amount", presence("notSet")))

	explicitNull := triggerMatches(t, s, notSet, map[string]any{"amount": nil}, fixtures.ContactAliceID)
	missing := triggerMatches(t, s, notSet, map[string]any{}, fixtures.ContactAliceID)

	assert.True(t, explicitNull, "an explicit null is not set")
	assert.False(t, missing, "a missing key is false, not unset")
}

func TestPropertyTestFoldsIntoTheRestOfTheTree(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.OscorpID)
	anna := env.DB.Contact.Query().Where(contact.Email(fixtures.OscorpAnnaEmail)).OnlyX(context.Background()).ID
	props := map[string]any{"url": "https://x.test/pricing"}
	hit := property("url", text("contains", "pricing"))
	miss := property("url", text("contains", "docs"))
	annaRow := attribute("first_name", text("eq", "Anna"))
	or := conditiontree.Rule{Group: groupOf(conditiontree.Or, false, miss, annaRow)}
	not := conditiontree.Rule{Group: groupOf(conditiontree.And, true, miss)}
	notHit := conditiontree.Rule{Group: groupOf(conditiontree.And, true, hit)}

	assert.True(t, triggerMatches(t, s, ruleOf(hit, annaRow), props, anna))
	assert.False(t, triggerMatches(t, s, ruleOf(miss, annaRow), props, anna))
	assert.True(t, triggerMatches(t, s, or, props, anna))
	assert.True(t, triggerMatches(t, s, not, props, anna), "not of a false leaf holds")
	assert.False(t, triggerMatches(t, s, notHit, props, anna), "not of a true leaf holds for no one")
	assert.True(t, triggerMatches(t, s, ruleOf(opsGroup(hit)), props, anna), "inside a nested group")
}

// Any key is valid on save: nothing says which properties an Event will carry. A property test
// with no key is refused.
func TestARulesPropertyTestTakesAnyKey(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	_, anyKey := condition.New(time.Now).RulePredicate(ctx, s, ruleOf(property("amount", text("eq", "10"))), nil)
	_, noKey := condition.New(time.Now).RulePredicate(ctx, s, ruleOf(property("", text("eq", "10"))), nil)

	require.NoError(t, anyKey)
	require.ErrorContains(t, noKey, "property key is required")
}

// asProperty is an attribute as an Event property: its text, or JSON null when the attribute is null.
func asProperty(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}
