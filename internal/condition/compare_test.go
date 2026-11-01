package condition_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent/customfield"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func numberLeaf(field string, op siteapi.SiteConditionNumberOperator, value float64) condition.NumberLeaf {
	return condition.NumberLeaf{Kind: "number", Field: field, Operator: string(op), Value: &value}
}

func dateAt(field string, op siteapi.SiteConditionDateOperator, at time.Time) condition.DateLeaf {
	return condition.DateLeaf{Kind: "date", Field: field, Operator: string(op), At: at}
}

func dateWithin(field string, days int) condition.DateWithinLeaf {
	return condition.DateWithinLeaf{Kind: "dateWithin", Field: field, WithinDays: days}
}

const (
	score  = "custom:" + fixtures.CustomFieldWonkaScoreKey
	seenAt = "custom:" + fixtures.CustomFieldWonkaSeenAtKey
)

// Wonka's scores: Ten 10, NineAndAHalf 9.5, Zulu 1, Minus -2.5; Text holds "10", Flag true, Null
// null, List [10], and None and Offset have no score at all. None of those six is a number.
func TestNumberLeafComparesANumberCustomField(t *testing.T) {
	env := testhelper.Setup(t)
	const ten, nine, zulu, minus = fixtures.WonkaTenEmail, fixtures.WonkaNineAndAHalfEmail, fixtures.WonkaZuluEmail, fixtures.WonkaMinusEmail

	for name, tc := range map[string]struct {
		leaf condition.NumberLeaf
		want []string
	}{
		"greater than":                   {numberLeaf(score, siteapi.SiteConditionNumberOperatorGt, 9.5), []string{ten}},
		"greater than or equal":          {numberLeaf(score, siteapi.SiteConditionNumberOperatorGte, 9.5), []string{nine, ten}},
		"less than":                      {numberLeaf(score, siteapi.SiteConditionNumberOperatorLt, 0), []string{minus}},
		"less than or equal":             {numberLeaf(score, siteapi.SiteConditionNumberOperatorLte, 10), []string{minus, nine, ten, zulu}},
		"only numbers, whatever the bar": {numberLeaf(score, siteapi.SiteConditionNumberOperatorGt, -1000), []string{minus, nine, ten, zulu}},
	} {
		got := emailsIn(t, env, fixtures.WonkaID, opsGroup(tc.leaf))
		assert.Equal(t, tc.want, got, name)
	}
}

// A Contact whose value is not a number fails the leaf, so the negated leaf holds for it, as it
// does for a Rule's folded trigger leaf.
func TestNotOfANumberLeafHoldsForAValueThatIsNotANumber(t *testing.T) {
	env := testhelper.Setup(t)
	not := condition.Group{Kind: "group", Combinator: "and", Not: true, Rules: marshal(numberLeaf(score, siteapi.SiteConditionNumberOperatorGt, -1000))}

	got := emailsIn(t, env, fixtures.WonkaID, not)

	assert.Equal(t, []string{
		fixtures.WonkaFlagEmail, fixtures.WonkaListEmail, fixtures.WonkaNoneEmail, fixtures.WonkaNullEmail,
		fixtures.WonkaOffsetEmail, fixtures.WonkaTextEmail,
	}, got)
}

// Wonka's seen_at: Ten 3 days ago, NineAndAHalf 10, Minus 40, Zulu and Offset in 2020, List 2
// days to come; Text holds "soon", Flag a bare date, Null a date-time without an offset, and None
// nothing. Only a date-time with an offset (a Z included) is a moment.
func TestDateLeafTestsADatetimeCustomField(t *testing.T) {
	env := testhelper.Setup(t)
	const ten, nine, minus, list = fixtures.WonkaTenEmail, fixtures.WonkaNineAndAHalfEmail, fixtures.WonkaMinusEmail, fixtures.WonkaListEmail
	const zulu, offset = fixtures.WonkaZuluEmail, fixtures.WonkaOffsetEmail
	fiveDaysAgo := time.Now().AddDate(0, 0, -5)

	for name, tc := range map[string]struct {
		leaf any
		want []string
	}{
		"before":                     {dateAt(seenAt, siteapi.SiteConditionDateOperatorBefore, fiveDaysAgo), []string{minus, nine, offset, zulu}},
		"after, a moment to come":    {dateAt(seenAt, siteapi.SiteConditionDateOperatorAfter, fiveDaysAgo), []string{list, ten}},
		"within a week":              {dateWithin(seenAt, 7), []string{ten}},
		"within a month":             {dateWithin(seenAt, 30), []string{nine, ten}},
		"a window past every moment": {dateWithin(seenAt, math.MaxInt32), []string{minus, nine, offset, ten, zulu}},
	} {
		got := emailsIn(t, env, fixtures.WonkaID, opsGroup(tc.leaf))
		assert.Equal(t, tc.want, got, name)
	}
}

// A moment is compared as an instant whatever its offset: Offset (2019-12-31 22:00:00.5 UTC) is
// before midnight UTC of 2020 and Zulu is that midnight. Before is strictly earlier and after is at
// or later, so Zulu is after midnight and not before it.
func TestDateLeafReadsAZuluAndAnOffsetMoment(t *testing.T) {
	env := testhelper.Setup(t)
	midnight := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	window := opsGroup(
		dateAt(seenAt, siteapi.SiteConditionDateOperatorAfter, midnight.Add(-3*time.Hour)),
		dateAt(seenAt, siteapi.SiteConditionDateOperatorBefore, midnight.Add(time.Hour)),
	)

	before := emailsIn(t, env, fixtures.WonkaID, opsGroup(dateAt(seenAt, siteapi.SiteConditionDateOperatorBefore, midnight)))
	after := emailsIn(t, env, fixtures.WonkaID, opsGroup(dateAt(seenAt, siteapi.SiteConditionDateOperatorAfter, midnight)))
	inWindow := emailsIn(t, env, fixtures.WonkaID, window)

	assert.Equal(t, []string{fixtures.WonkaOffsetEmail}, before)
	assert.Contains(t, after, fixtures.WonkaZuluEmail)
	assert.NotContains(t, after, fixtures.WonkaOffsetEmail)
	assert.Equal(t, []string{fixtures.WonkaOffsetEmail, fixtures.WonkaZuluEmail}, inWindow)
}

// The moment of before and after keeps its offset: the same instant written in another zone
// selects the same Contacts.
func TestDateLeafComparesInstantsWhateverTheirZone(t *testing.T) {
	env := testhelper.Setup(t)
	at := time.Now().AddDate(0, 0, -5)
	elsewhere := at.In(time.FixedZone("UTC-11", -11*60*60))

	utc := emailsIn(t, env, fixtures.WonkaID, opsGroup(dateAt(seenAt, siteapi.SiteConditionDateOperatorBefore, at.UTC())))
	zoned := emailsIn(t, env, fixtures.WonkaID, opsGroup(dateAt(seenAt, siteapi.SiteConditionDateOperatorBefore, elsewhere)))

	assert.Equal(t, utc, zoned)
}

// The numeric family is one table for a number Custom field (SQL) and a number property of the
// triggering Event (Go): for any operator, operand and Contact, alone or negated, the two agree.
func TestNumberLeavesAgreeOnTheContactSideAndTheEventSide(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.WonkaID)
	contacts, err := s.Contact().Query().All(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, contacts)

	rapid.Check(t, func(rt *rapid.T) {
		op := rapid.SampledFrom(siteapi.SiteConditionNumberOperator("").AllValues()).Draw(rt, "operator")
		want := rapid.SampledFrom([]float64{-1000, -2.5, 0, 9, 9.5, 10, 10.5}).Draw(rt, "operand")
		not := rapid.Bool().Draw(rt, "not")
		c := rapid.SampledFrom(contacts).Draw(rt, "contact")
		// The Contact's Custom fields are the Event's properties: the same value under the same key.
		onContact := condition.Group{Kind: "group", Combinator: "and", Not: not, Rules: marshal(numberLeaf(score, op, want))}
		onEvent := condition.Group{Kind: "group", Combinator: "and", Not: not, Rules: marshal(numberLeaf("trigger:"+fixtures.CustomFieldWonkaScoreKey, op, want))}

		viaSQL := triggerMatches(rt, s, onContact, nil, c.ID)
		viaGo := triggerMatches(rt, s, onEvent, c.CustomFields, c.ID)

		assert.Equal(rt, viaSQL, viaGo, "not=%v %s %v on %v", not, op, want, c.CustomFields)
	})
}

// A collected Event keeps its numbers as json.Number; a stored one decodes them to float64.
func TestTriggerNumberLeafReadsEitherDecodingOfANumber(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	atLeastNine := opsGroup(numberLeaf("trigger:score", siteapi.SiteConditionNumberOperatorGte, 9))

	collected := triggerMatches(t, s, atLeastNine, map[string]any{"score": json.Number("9.5")}, fixtures.ContactAliceID)
	stored := triggerMatches(t, s, atLeastNine, map[string]any{"score": 9.0}, fixtures.ContactAliceID)
	text := triggerMatches(t, s, atLeastNine, map[string]any{"score": "10"}, fixtures.ContactAliceID)

	assert.True(t, collected)
	assert.True(t, stored)
	assert.False(t, text, "a number written as text is not a number")
}

func TestTriggerNumberLeafIsValidOnlyInARule(t *testing.T) {
	env := testhelper.Setup(t)
	g := opsGroup(numberLeaf("trigger:score", siteapi.SiteConditionNumberOperatorGt, 1))

	_, err := condition.Compile(context.Background(), env.DB.Scoped(fixtures.AcmeID), g)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "only in a Rule")
}

func TestCompileRefusesANumberLeafThatDoesNotFit(t *testing.T) {
	env := testhelper.Setup(t)
	withoutValue := numberLeaf(score, siteapi.SiteConditionNumberOperatorGt, 1)
	withoutValue.Value = nil

	for name, tc := range map[string]struct {
		leaf condition.NumberLeaf
		msg  string
	}{
		"a string field":      {numberLeaf("custom:"+fixtures.CustomFieldWonkaPlanKey, siteapi.SiteConditionNumberOperatorGt, 1), "is of type string, not number"},
		"a datetime field":    {numberLeaf(seenAt, siteapi.SiteConditionNumberOperatorGt, 1), "is of type datetime, not number"},
		"an unknown field":    {numberLeaf("custom:nobody", siteapi.SiteConditionNumberOperatorGt, 1), "unknown custom field"},
		"a contact attribute": {numberLeaf("email", siteapi.SiteConditionNumberOperatorGt, 1), "takes a custom field of type number or a trigger property"},
		"the tag field":       {numberLeaf("tag", siteapi.SiteConditionNumberOperatorGt, 1), "takes a custom field of type number or a trigger property"},
		"a text operator":     {numberLeaf(score, "eq", 1), "unsupported operator"},
		"a date operator":     {numberLeaf(score, "before", 1), "unsupported operator"},
		"no value":            {withoutValue, "value is required"},
	} {
		_, err := condition.Compile(context.Background(), env.DB.Scoped(fixtures.WonkaID), opsGroup(tc.leaf))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.msg, name)
		assert.Contains(t, err.Error(), "rule 0", "%s: the error names the offending rule", name)
	}
}

// Anna was born on 1990-05-01, Bella on 1985-11-20, and Andy has no birthday.
func TestDateLeafTestsTheBirthday(t *testing.T) {
	env := testhelper.Setup(t)
	match := func(leaf any) []string { return emailsIn(t, env, fixtures.OscorpID, opsGroup(leaf)) }
	cut := time.Date(1988, time.January, 1, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, []string{fixtures.OscorpBellaEmail}, match(dateAt("birthday", siteapi.SiteConditionDateOperatorBefore, cut)))
	assert.Equal(t, []string{fixtures.OscorpAnnaEmail}, match(dateAt("birthday", siteapi.SiteConditionDateOperatorAfter, cut)))
	assert.Empty(t, match(dateWithin("birthday", 7)), "no birthday falls in the last week")
}

func TestCompileRefusesADateLeafThatDoesNotFit(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()

	for name, tc := range map[string]struct {
		leaf any
		msg  string
	}{
		"a number field":            {dateWithin(score, 7), "is of type number, not datetime"},
		"a string field":            {dateAt("custom:"+fixtures.CustomFieldWonkaPlanKey, siteapi.SiteConditionDateOperatorBefore, now), "is of type string, not datetime"},
		"an unknown field":          {dateWithin("custom:nobody", 7), "unknown custom field"},
		"a trigger property":        {dateWithin("trigger:at", 7), "takes a custom field of type datetime"},
		"a contact attribute":       {dateAt("email", siteapi.SiteConditionDateOperatorAfter, now), "takes a custom field of type datetime"},
		"a number operator":         {dateAt(seenAt, "gt", now), "unsupported operator"},
		"a window of no days":       {dateWithin(seenAt, 0), "withinDays must be 1 or more"},
		"a window of negative days": {dateWithin(seenAt, -1), "withinDays must be 1 or more"},
	} {
		_, err := condition.Compile(context.Background(), env.DB.Scoped(fixtures.WonkaID), opsGroup(tc.leaf))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.msg, name)
		assert.Contains(t, err.Error(), "rule 0", "%s: the error names the offending rule", name)
	}
}

// A rule leaf on a Custom field takes only the operators that fit the field's type, the ones the
// API lists as its ruleOperators.
func TestCompileRefusesARuleOperatorThatDoesNotFitTheCustomFieldType(t *testing.T) {
	env := testhelper.Setup(t)

	for name, leaf := range map[string]condition.Leaf{
		"contains on a number":     rule(score, "contains", "1"),
		"eq on a datetime":         rule(seenAt, "eq", "2020-01-01T00:00:00Z"),
		"contains on a datetime":   rule(seenAt, "contains", "2020"),
		"beginsWith on a string":   rule("custom:"+fixtures.CustomFieldWonkaPlanKey, "beginsWith", "p"),
		"a number operator on one": rule(score, "gt", "1"),
	} {
		_, err := condition.Compile(context.Background(), env.DB.Scoped(fixtures.WonkaID), opsGroup(leaf))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "unsupported operator", name)
	}
}

// What fits is what compiles: every operator listed for a type compiles on a field of that type.
func TestEveryListedRuleOperatorCompiles(t *testing.T) {
	env := testhelper.Setup(t)
	fields := map[customfield.Type]string{
		customfield.TypeString:   "custom:" + fixtures.CustomFieldWonkaPlanKey,
		customfield.TypeNumber:   score,
		customfield.TypeDatetime: seenAt,
	}

	for typ, field := range fields {
		for _, op := range condition.RuleOperators(typ) {
			_, err := condition.Compile(context.Background(), env.DB.Scoped(fixtures.WonkaID), opsGroup(rule(field, op, "x")))
			require.NoError(t, err, "%s on %s", op, typ)
		}
	}
}

// A Rule refuses a date leaf on a trigger property too: an Event's properties have no type.
func TestRuleRefusesADateLeafOnATriggerProperty(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)

	_, err := condition.RulePredicate(context.Background(), s, encode(t, opsGroup(dateWithin("trigger:at", 7))), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "takes a custom field of type datetime")
}
