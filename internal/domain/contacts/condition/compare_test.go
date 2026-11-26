package condition_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	score  = fixtures.CustomFieldWonkaScoreKey
	seenAt = fixtures.CustomFieldWonkaSeenAtKey
)

// Wonka's scores: Ten 10, NineAndAHalf 9.5, Zulu 1, Minus -2.5; Text holds "10", Flag true, Null
// null, List [10], and None and Offset have no score at all. None of those six is a number.
func TestNumberLeafComparesANumberCustomField(t *testing.T) {
	env := testhelper.Setup(t)
	const ten, nine, zulu, minus = fixtures.WonkaTenEmail, fixtures.WonkaNineAndAHalfEmail, fixtures.WonkaZuluEmail, fixtures.WonkaMinusEmail

	for name, tc := range map[string]struct {
		test leaf
		want []string
	}{
		"greater than":                   {number("gt", 9.5), []string{ten}},
		"greater than or equal":          {number("gte", 9.5), []string{nine, ten}},
		"less than":                      {number("lt", 0), []string{minus}},
		"less than or equal":             {number("lte", 10), []string{minus, nine, ten, zulu}},
		"only numbers, whatever the bar": {number("gt", -1000), []string{minus, nine, ten, zulu}},
	} {
		got := emailsIn(t, env, fixtures.WonkaID, segmentOf(customField(score, tc.test)))
		assert.Equal(t, tc.want, got, name)
	}
}

// A Contact whose value is not a number fails the leaf, so the negated leaf holds for it, as it
// does for a Rule's folded property test.
func TestNotOfANumberLeafHoldsForAValueThatIsNotANumber(t *testing.T) {
	env := testhelper.Setup(t)
	not := conditiontree.Segment{Group: groupOf(conditiontree.And, true, customField(score, number("gt", -1000)))}

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
	const flag, null = fixtures.WonkaFlagEmail, fixtures.WonkaNullEmail // a bare date, a date-time without an offset
	fiveDaysAgo := time.Now().AddDate(0, 0, -5)

	for name, tc := range map[string]struct {
		test leaf
		want []string
	}{
		"before":                     {moment("before", fiveDaysAgo), []string{flag, minus, nine, null, offset, zulu}},
		"after, a moment to come":    {moment("after", fiveDaysAgo), []string{list, ten}},
		"within a week":              {withinLast(7), []string{ten}},
		"within a month":             {withinLast(30), []string{nine, ten}},
		"a window past every moment": {withinLast(math.MaxInt32), []string{flag, minus, nine, null, offset, ten, zulu}},
	} {
		got := emailsIn(t, env, fixtures.WonkaID, segmentOf(customField(seenAt, tc.test)))
		assert.Equal(t, tc.want, got, name)
	}
}

// A moment is compared as an instant whatever its offset: Offset (2019-12-31 22:00:00.5 UTC) is
// before midnight UTC of 2020 and Zulu is that midnight. Before is strictly earlier and after is at
// or later, so Zulu is after midnight and not before it.
func TestDateLeafReadsAZuluAndAnOffsetMoment(t *testing.T) {
	env := testhelper.Setup(t)
	midnight := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	window := segmentOf(
		customField(seenAt, moment("after", midnight.Add(-3*time.Hour))),
		customField(seenAt, moment("before", midnight.Add(time.Hour))),
	)

	before := emailsIn(t, env, fixtures.WonkaID, segmentOf(customField(seenAt, moment("before", midnight))))
	after := emailsIn(t, env, fixtures.WonkaID, segmentOf(customField(seenAt, moment("after", midnight))))
	inWindow := emailsIn(t, env, fixtures.WonkaID, window)

	assert.Equal(t, []string{fixtures.WonkaOffsetEmail}, before)
	assert.Contains(t, after, fixtures.WonkaZuluEmail)
	assert.NotContains(t, after, fixtures.WonkaOffsetEmail)
	assert.Equal(t, []string{fixtures.WonkaOffsetEmail, fixtures.WonkaZuluEmail}, inWindow)
}

// A value without an offset is a moment in the Workspace time zone (ADR 0087): Flag's bare date
// 2026-01-01 is the start of that day there, so against 23:00 UTC of the evening before it is after
// that bound in UTC and before it in Tokyo, where the day began at 15:00 UTC of the 31st.
func TestDateLeafReadsAValueWithoutAnOffsetInTheWorkspaceTimeZone(t *testing.T) {
	env := testhelper.Setup(t)
	eve := time.Date(2025, time.December, 31, 23, 0, 0, 0, time.UTC)
	beforeEve := segmentOf(customField(seenAt, moment("before", eve)))
	utc := emailsIn(t, env, fixtures.WonkaID, beforeEve)
	err := env.DB.Workspace.UpdateOneID(fixtures.WonkaID).SetTimezone("Asia/Tokyo").Exec(t.Context())
	require.NoError(t, err)

	tokyo := emailsIn(t, env, fixtures.WonkaID, beforeEve)

	assert.NotContains(t, utc, fixtures.WonkaFlagEmail)
	assert.Contains(t, tokyo, fixtures.WonkaFlagEmail)
	assert.NotContains(t, tokyo, fixtures.WonkaTextEmail, "text is no moment")
}

// The moment of before and after keeps its offset: the same instant written in another zone
// selects the same Contacts.
func TestDateLeafComparesInstantsWhateverTheirZone(t *testing.T) {
	env := testhelper.Setup(t)
	at := time.Now().AddDate(0, 0, -5)
	elsewhere := at.In(time.FixedZone("UTC-11", -11*60*60))

	utc := emailsIn(t, env, fixtures.WonkaID, segmentOf(customField(seenAt, moment("before", at.UTC()))))
	zoned := emailsIn(t, env, fixtures.WonkaID, segmentOf(customField(seenAt, moment("before", elsewhere))))

	assert.Equal(t, utc, zoned)
}

// A collected Event keeps its numbers as json.Number; a stored one decodes them to float64.
func TestPropertyNumberTestReadsEitherDecodingOfANumber(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	atLeastNine := ruleOf(property("score", number("gte", 9)))

	collected := triggerMatches(t, s, atLeastNine, map[string]any{"score": json.Number("9.5")}, fixtures.ContactAliceID)
	stored := triggerMatches(t, s, atLeastNine, map[string]any{"score": 9.0}, fixtures.ContactAliceID)
	text := triggerMatches(t, s, atLeastNine, map[string]any{"score": "10"}, fixtures.ContactAliceID)

	assert.True(t, collected)
	assert.True(t, stored)
	assert.False(t, text, "a number written as text is not a number")
}

// A number test on a Custom field that is not a number, or one the Workspace does not have, is
// refused, naming its entry.
func TestCompileRefusesANumberTestThatDoesNotFit(t *testing.T) {
	env := testhelper.Setup(t)

	for name, tc := range map[string]struct {
		key string
		msg string
	}{
		"a string field":   {fixtures.CustomFieldWonkaPlanKey, `of type string takes no number test "gt"`},
		"a datetime field": {seenAt, `of type datetime takes no number test "gt"`},
		"an unknown field": {"nobody", "unknown custom field"},
	} {
		_, err := condition.New(time.Now).Compile(t.Context(), env.DB.Scoped(fixtures.WonkaID), segmentOf(customField(tc.key, number("gt", 1))))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.msg, name)
		assert.Contains(t, err.Error(), "entry 0", "%s: the error names the offending entry", name)
	}
}

// Anna was born on 1990-05-01, Bella on 1985-11-20, and Andy has no birthday.
func TestBirthdayLeafPlacesTheDateOfBirth(t *testing.T) {
	env := testhelper.Setup(t)
	match := func(test leaf) []string { return emailsIn(t, env, fixtures.OscorpID, segmentOf(birthday(test))) }
	cut := time.Date(1988, time.January, 1, 0, 0, 0, 0, time.UTC)

	before := match(moment("before", cut))
	after := match(moment("after", cut))
	lastWeek := match(withinLast(7))

	assert.Equal(t, []string{fixtures.OscorpBellaEmail}, before)
	assert.Equal(t, []string{fixtures.OscorpAnnaEmail}, after)
	assert.Empty(t, lastWeek, "no birthday falls in the last week")
}

// A moment test on a Custom field that is not a datetime, one the Workspace does not have, or a
// window of no days, is refused, naming its entry.
func TestCompileRefusesAMomentTestThatDoesNotFit(t *testing.T) {
	env := testhelper.Setup(t)
	now := time.Now()

	for name, tc := range map[string]struct {
		leaf leaf
		msg  string
	}{
		"a number field":            {customField(score, withinLast(7)), "of type number takes no withinLast test"},
		"a string field":            {customField(fixtures.CustomFieldWonkaPlanKey, moment("before", now)), `of type string takes no moment test "before"`},
		"an unknown field":          {customField("nobody", withinLast(7)), "unknown custom field"},
		"a window of no days":       {customField(seenAt, withinLast(0)), "days must be at least 1"},
		"a window of negative days": {customField(seenAt, withinLast(-1)), "days must be at least 1"},
	} {
		_, err := condition.New(time.Now).Compile(t.Context(), env.DB.Scoped(fixtures.WonkaID), segmentOf(tc.leaf))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.msg, name)
		assert.Contains(t, err.Error(), "entry 0", "%s: the error names the offending entry", name)
	}
}

// A text or presence test on a Custom field takes only the operators that fit the field's type,
// the ones the API lists as its conditionTests.
func TestCompileRefusesATestOperatorThatDoesNotFitTheCustomFieldType(t *testing.T) {
	env := testhelper.Setup(t)

	for name, tc := range map[string]struct {
		leaf leaf
		msg  string
	}{
		"contains on a number":   {customField(score, text("contains", "1")), `takes no text test "contains"`},
		"eq on a datetime":       {customField(seenAt, text("eq", "2020-01-01T00:00:00Z")), `takes no text test "eq"`},
		"contains on a datetime": {customField(seenAt, text("contains", "2020")), `takes no text test "contains"`},
		"beginsWith on a string": {customField(fixtures.CustomFieldWonkaPlanKey, text("beginsWith", "p")), `takes no text test "beginsWith"`},
		"not set on a number":    {customField(score, presence("notSet")), `takes no presence test "notSet"`},
	} {
		_, err := condition.New(time.Now).Compile(t.Context(), env.DB.Scoped(fixtures.WonkaID), segmentOf(tc.leaf))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.msg, name)
	}
}

// What fits is what compiles: every test and operator listed for a type compiles on a field of
// that type.
func TestEveryListedTestCompiles(t *testing.T) {
	env := testhelper.Setup(t)
	keys := map[customfield.Type]string{
		customfield.TypeString:   fixtures.CustomFieldWonkaPlanKey,
		customfield.TypeNumber:   score,
		customfield.TypeDatetime: seenAt,
	}

	for typ, key := range keys {
		for _, test := range listedTests(condition.CustomFieldTests(typ)) {
			_, err := condition.New(time.Now).Compile(t.Context(), env.DB.Scoped(fixtures.WonkaID), segmentOf(customField(key, test)))
			require.NoError(t, err, "%v on %s", test, typ)
		}
	}
}

// listedTests is a test of every operator the listing names, and withinLast when it takes it.
func listedTests(tests condition.Tests) []leaf {
	var out []leaf
	for _, op := range tests.Text {
		out = append(out, text(string(op), "x"))
	}
	for _, op := range tests.Number {
		out = append(out, number(string(op), 1))
	}
	for _, op := range tests.Moment {
		out = append(out, moment(string(op), time.Now()))
	}
	for _, op := range tests.Presence {
		out = append(out, presence(string(op)))
	}
	if tests.WithinLast {
		out = append(out, withinLast(7))
	}
	return out
}
