package condition_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestPlainColumnOperators(t *testing.T) {
	env := testhelper.Setup(t)
	const anna, andy, bella = fixtures.OscorpAnnaEmail, fixtures.OscorpAndyEmail, fixtures.OscorpBellaEmail

	for name, tc := range map[string]struct {
		leaf leaf
		want []string
	}{
		"equals":      {attribute("first_name", text("eq", "Anna")), []string{anna}},
		"not equals":  {attribute("first_name", text("neq", "Anna")), []string{andy, bella}},
		"contains":    {attribute("first_name", text("contains", "ELL")), []string{bella}},
		"begins with": {attribute("email", text("beginsWith", "an")), []string{andy, anna}},
		"not set":     {attribute("phone", presence("notSet")), []string{andy}},
		"set":         {attribute("phone", presence("set")), []string{anna, bella}},
		"a country":   {attribute("country", text("eq", "DE")), []string{anna}},
		"no country":  {attribute("country", presence("notSet")), []string{andy}},
		"a city":      {attribute("city", text("eq", "Berlin")), []string{anna}},
		"a locale":    {attribute("locale", text("beginsWith", "fr")), []string{bella}},
	} {
		got := emailsIn(t, env, fixtures.OscorpID, segmentOf(tc.leaf))
		assert.Equal(t, tc.want, got, name)
	}
}

func TestCustomFieldOperators(t *testing.T) {
	env := testhelper.Setup(t)
	match := func(l leaf) []string { return emailsIn(t, env, fixtures.InitrodeID, segmentOf(l)) }

	assert.Equal(t, []string{fixtures.InitrodeOneEmail}, match(customField("plan", text("eq", "pro"))))
	assert.Equal(t, []string{fixtures.InitrodeTwoEmail}, match(customField("plan", text("neq", "pro"))))
	assert.Equal(t, []string{fixtures.InitrodeOneEmail, fixtures.InitrodeTwoEmail}, match(customField("plan", presence("set"))))
	// contains on a Custom field is membership of a JSON array, not substring.
	assert.Equal(t, []string{fixtures.InitrodeOneEmail}, match(customField("tags", text("contains", "beta"))))
	assert.Empty(t, match(customField("tags", text("contains", "gamma"))))
}

// The key of a Custom field test is data, never SQL: quotes in it are matched literally
// and an injection payload is just a key nobody has.
func TestCustomFieldKeyIsABoundArgument(t *testing.T) {
	env := testhelper.Setup(t)
	match := func(l leaf) []string { return emailsIn(t, env, fixtures.InitrodeID, segmentOf(l)) }

	assert.Equal(t, []string{fixtures.InitrodeQuoteEmail}, match(customField("it's", text("eq", "here"))))

	// Even a declared key that looks like SQL is data: the catalogue accepts any key
	// ingest created, and it only ever reaches the query as one quoted literal.
	payload := fixtures.CustomFieldInitrodeInjectionKey
	// Four representative tests for the injection probe.
	for name, test := range map[string]leaf{
		"equals": text("eq", "a"), "differs": text("neq", "a"), "holds": text("contains", "a"), "is set": presence("set"),
	} {
		assert.Empty(t, match(customField(payload, test)), name)
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
		_, err := condition.New(time.Now).Compile(t.Context(), ws, segmentOf(customField(key, text("eq", "a"))))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "unknown custom field", name)
	}
}

func TestEmptyGroupsMatchEveryoneUnlessNegated(t *testing.T) {
	env := testhelper.Setup(t)

	assert.Equal(t, []string{
		fixtures.InitrodeOneEmail, fixtures.InitrodeQuoteEmail, fixtures.InitrodeThreeEmail, fixtures.InitrodeTwoEmail,
	}, emailsIn(t, env, fixtures.InitrodeID, segmentOf()))
	assert.Empty(t, emailsIn(t, env, fixtures.InitrodeID, conditiontree.Segment{Group: groupOf(conditiontree.And, true)}),
		"an empty negated group matches no one")
}

// What the tree's type holds but its data does not: a blank Tag name, a Custom field test its type
// does not take, a Custom field leaf with no key. Each names its entry.
func TestCompileRefusesWhatTheTreeDoesNotAllow(t *testing.T) {
	env := testhelper.Setup(t)
	for name, tc := range map[string]struct {
		leaf leaf
		msg  string
	}{
		"blank tag name":                  {tagLeaf("   ", "has"), "tag name is required"},
		"beginsWith on a Custom field":    {customField("plan", text("beginsWith", "p")), `takes no text test "beginsWith"`},
		"a Custom field leaf with no key": {customField("", text("eq", "x")), "custom field key is required"},
		"not set on a Custom field":       {customField("plan", presence("notSet")), `takes no presence test "notSet"`},
	} {
		_, err := condition.New(time.Now).Compile(t.Context(), env.DB.Scoped(fixtures.AcmeID), segmentOf(tc.leaf))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.msg, name)
		assert.Contains(t, err.Error(), "entry 0", "%s: the error names the offending entry", name)
	}
}

func TestCompileRefusesAnEventLeafOutOfRange(t *testing.T) {
	env := testhelper.Setup(t)
	withoutTimes := eventLeaf("signup", "atLeast", 1, 0)
	delete(withoutTimes, "times")
	zeroWindow := eventLeaf("signup", "atLeast", 1, 0)
	zeroWindow["withinDays"] = 0

	for name, tc := range map[string]struct {
		leaf leaf
		msg  string
	}{
		"no action":                {eventLeaf("", "atLeast", 1, 0), "event action is required"},
		"a blank action":           {eventLeaf("  ", "atLeast", 1, 0), "event action is required"},
		"no times":                 {withoutTimes, "times is required"},
		"at least none":            {eventLeaf("signup", "atLeast", 0, 0), "atLeast takes times of 1 or more"},
		"at most a negative count": {eventLeaf("signup", "atMost", -1, 0), "times must be at least 0"},
		"a zero window":            {zeroWindow, "withinDays must be at least 1"},
		"a negative window":        {eventLeaf("signup", "atLeast", 1, -1), "withinDays must be at least 1"},
	} {
		_, err := condition.New(time.Now).Compile(t.Context(), env.DB.Scoped(fixtures.AcmeID), segmentOf(tc.leaf))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), tc.msg, name)
		assert.Contains(t, err.Error(), "entry 0", "%s: the error names the offending entry", name)
	}
}

func TestCompileRefusesMalformedEntries(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	ws := env.DB.Scoped(fixtures.AcmeID)
	compile := func(entry string) error {
		g := conditiontree.Group{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{json.RawMessage(entry)}}
		_, err := condition.New(time.Now).Compile(ctx, ws, conditiontree.Segment{Group: g})
		return err
	}

	notAnObject := compile(`42`)
	entriesNotAList := compile(`{"kind":"group","combinator":"or","entries":"nope"}`)
	valueNotText := compile(`{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"eq","value":{"a":1}}}`)
	noKind := compile(`{"attribute":"email","test":{"kind":"text","operator":"eq","value":"x"}}`)
	_, xor := condition.New(time.Now).Compile(ctx, ws, conditiontree.Segment{Group: groupOf("xor", false)})
	_, nested := condition.New(time.Now).Compile(ctx, ws, segmentOf(opsGroup(tagLeaf(" ", "has"))))

	require.ErrorContains(t, notAnObject, "malformed entry")
	require.ErrorContains(t, entriesNotAList, "malformed group")
	require.ErrorContains(t, valueNotText, "text test value")
	require.ErrorContains(t, noKind, "unknown entry kind", "a kind is never guessed from the fields")
	require.ErrorContains(t, xor, "unknown combinator", "an unknown combinator is not read as and")
	require.ErrorContains(t, nested, "entry 0: entry 0: tag name is required", "an error deep in a nested group names its path")
}

// A group without its entries is refused at any depth, never read as the empty group that matches
// every Contact.
func TestCompileRefusesAGroupWithoutEntries(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	ws := env.DB.Scoped(fixtures.AcmeID)
	var root conditiontree.Segment
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and"}`), &root))
	nestedGroup := conditiontree.Group{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{
		json.RawMessage(`{"kind":"group","combinator":"or","entries":null}`),
	}}

	_, atTheRoot := condition.New(time.Now).Compile(ctx, ws, root)
	_, nested := condition.New(time.Now).Compile(ctx, ws, conditiontree.Segment{Group: nestedGroup})

	require.ErrorContains(t, atTheRoot, "group entries are required")
	require.ErrorContains(t, nested, "entry 0: group entries are required")
}

func TestCompileRefusesInvalidConditions(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	ws := env.DB.Scoped(fixtures.AcmeID)

	_, err := condition.New(time.Now).Compile(ctx, ws, testhelper.SegmentCondition(t, `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"nope","test":{"kind":"text","operator":"eq","value":"x"}}]}`))
	require.Error(t, err)

	_, err = condition.New(time.Now).Compile(ctx, ws, conditiontree.Segment{})
	require.Error(t, err, "a document that is no group")

	p, err := condition.New(time.Now).Compile(ctx, ws, conditiontree.Segment{Group: conditiontree.Empty()})
	require.NoError(t, err)
	assert.NotNil(t, p, "the empty group matches everyone")
}
