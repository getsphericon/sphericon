package condition_test

import (
	"context"
	"encoding/json"
	"testing"

	"entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// filterHolds compiles f and runs its path in Postgres on doc, a JSON object.
func filterHolds(t *testing.T, env *testhelper.TestEnv, f conditiontree.EventFilter, doc string) bool {
	t.Helper()
	path, err := condition.CompileEventFilter(f)
	require.NoError(t, err)
	on := condition.Document(json.RawMessage(doc))
	holds := predicate.Contact(func(s *sql.Selector) { s.Where(path.On(on)) })
	ok, err := env.DB.Scoped(fixtures.AcmeID).Contact().Query().Where(contact.ID(fixtures.ContactAliceID), holds).Exist(context.Background())
	require.NoError(t, err)
	return ok
}

// filterCase is an object a property test of the key k runs on, and whether the test holds there.
type filterCase struct {
	doc  string
	want bool
}

// otherTypes are the objects whose k is missing, or of a type no text, presence or number test
// takes as a match (null aside, which notSet takes): every test but notSet fails on them, so its
// negation holds. A list that holds a matching value is no match: a list is never unwrapped.
func otherTypes(cases map[string]filterCase) map[string]filterCase {
	for name, doc := range map[string]string{
		"a missing property": `{"other": "Vip"}`,
		"a boolean":          `{"k": true}`,
		"an object":          `{"k": {"k": "Vip"}}`,
		"a list":             `{"k": ["Vip", 10]}`,
	} {
		cases[name] = filterCase{doc: doc}
	}
	return cases
}

// operatorHolds runs the property test of k on every case, as it is and negated: the filter is
// two-valued, so the negation holds exactly where the test does not.
func operatorHolds(t *testing.T, test leaf, cases map[string]filterCase) {
	t.Helper()
	env := testhelper.Setup(t)
	for name, tc := range cases {
		plain := filterHolds(t, env, filterOf(prop("k", test)), tc.doc)
		negated := filterHolds(t, env, conditiontree.EventFilter{Group: groupOf(conditiontree.And, true, prop("k", test))}, tc.doc)

		assert.Equal(t, tc.want, plain, "%s: %s", name, tc.doc)
		assert.Equal(t, !tc.want, negated, "not, %s: %s", name, tc.doc)
	}
}

func TestFilterEqComparesText(t *testing.T) {
	operatorHolds(t, text("eq", "Vip"), otherTypes(map[string]filterCase{
		"the same text":       {`{"k": "Vip"}`, true},
		"another case":        {`{"k": "vip"}`, false},
		"another text":        {`{"k": "Vipp"}`, false},
		"a number of that":    {`{"k": 10}`, false},
		"null":                {`{"k": null}`, false},
		"other properties":    {`{"a": "x", "k": "Vip", "z": 1}`, true},
		"the text elsewhere":  {`{"kk": "Vip"}`, false},
		"a key with a dot":    {`{"k.x": "Vip"}`, false},
		"the text, not a key": {`{"Vip": "k"}`, false},
	}))
}

func TestFilterNeqComparesText(t *testing.T) {
	operatorHolds(t, text("neq", "Vip"), otherTypes(map[string]filterCase{
		"the same text": {`{"k": "Vip"}`, false},
		"another case":  {`{"k": "vip"}`, true},
		"another text":  {`{"k": "Gold"}`, true},
		"a number":      {`{"k": 10}`, false},
		"null":          {`{"k": null}`, false},
	}))
}

func TestFilterContainsIsACaseInsensitiveSubstring(t *testing.T) {
	operatorHolds(t, text("contains", "iP"), otherTypes(map[string]filterCase{
		"inside":         {`{"k": "a vip one"}`, true},
		"another case":   {`{"k": "VIP"}`, true},
		"the whole text": {`{"k": "ip"}`, true},
		"absent":         {`{"k": "gold"}`, false},
		"null":           {`{"k": null}`, false},
		"a number":       {`{"k": 10}`, false},
	}))
}

func TestFilterBeginsWithIsACaseSensitivePrefix(t *testing.T) {
	operatorHolds(t, text("beginsWith", "Vi"), otherTypes(map[string]filterCase{
		"a prefix":     {`{"k": "Vip"}`, true},
		"another case": {`{"k": "vip"}`, false},
		"inside":       {`{"k": "a Vip"}`, false},
		"null":         {`{"k": null}`, false},
		"a number":     {`{"k": 10}`, false},
	}))
}

// set is a text value and notSet an explicit null, as on a Contact's attribute, whose value is text
// or null: a missing property is neither.
func TestFilterSetIsAText(t *testing.T) {
	operatorHolds(t, presence("set"), otherTypes(map[string]filterCase{
		"a text":       {`{"k": "Vip"}`, true},
		"an empty one": {`{"k": ""}`, true},
		"null":         {`{"k": null}`, false},
		"a number":     {`{"k": 10}`, false},
	}))
}

func TestFilterNotSetIsAnExplicitNull(t *testing.T) {
	operatorHolds(t, presence("notSet"), otherTypes(map[string]filterCase{
		"null":     {`{"k": null}`, true},
		"a text":   {`{"k": "Vip"}`, false},
		"a number": {`{"k": 10}`, false},
	}))
}

func TestFilterGtComparesANumber(t *testing.T) {
	operatorHolds(t, number("gt", 9.5), otherTypes(map[string]filterCase{
		"above":            {`{"k": 10}`, true},
		"equal":            {`{"k": 9.5}`, false},
		"below":            {`{"k": -2}`, false},
		"a number as text": {`{"k": "10"}`, false},
		"null":             {`{"k": null}`, false},
	}))
}

func TestFilterGteComparesANumber(t *testing.T) {
	operatorHolds(t, number("gte", 9.5), otherTypes(map[string]filterCase{
		"above":            {`{"k": 10}`, true},
		"equal":            {`{"k": 9.50}`, true},
		"below":            {`{"k": 9.49}`, false},
		"a number as text": {`{"k": "10"}`, false},
		"null":             {`{"k": null}`, false},
	}))
}

func TestFilterLtComparesANumber(t *testing.T) {
	operatorHolds(t, number("lt", 0), otherTypes(map[string]filterCase{
		"below":            {`{"k": -2.5}`, true},
		"equal":            {`{"k": 0}`, false},
		"above":            {`{"k": 1e3}`, false},
		"a number as text": {`{"k": "-1"}`, false},
		"null":             {`{"k": null}`, false},
	}))
}

func TestFilterLteComparesANumber(t *testing.T) {
	operatorHolds(t, number("lte", 10), otherTypes(map[string]filterCase{
		"below":            {`{"k": -2.5}`, true},
		"equal":            {`{"k": 10.0}`, true},
		"above":            {`{"k": 10.01}`, false},
		"a number as text": {`{"k": "1"}`, false},
		"null":             {`{"k": null}`, false},
	}))
}

// A filter's groups combine their tests by and or by or, at any depth and under a negation; an
// empty group holds for every object and an empty negated group for none.
func TestFilterGroupsCombineTheirTests(t *testing.T) {
	env := testhelper.Setup(t)
	const doc = `{"status": "paid", "total": 120, "coupon": null}`
	paid := prop("status", text("eq", "paid"))
	failed := prop("status", text("eq", "failed"))
	big := prop("total", number("gte", 100))

	for name, tc := range map[string]struct {
		filter conditiontree.EventFilter
		want   bool
	}{
		"and of two that hold":  {filterOf(paid, big), true},
		"and with one that not": {filterOf(failed, big), false},
		"or with one that does": {conditiontree.EventFilter{Group: groupOf(conditiontree.Or, false, failed, big)}, true},
		"or of none that does":  {conditiontree.EventFilter{Group: groupOf(conditiontree.Or, false, failed)}, false},
		"not of a nested group": {filterOf(paid, groupOf(conditiontree.Or, true, failed, prop("coupon", presence("set")))), true},
		"a nested negation":     {filterOf(groupOf(conditiontree.And, true, paid)), false},
		"the empty group":       {filterOf(), true},
		"the empty or-group":    {conditiontree.EventFilter{Group: groupOf(conditiontree.Or, false)}, true},
		"empty and negated":     {conditiontree.EventFilter{Group: groupOf(conditiontree.And, true)}, false},
	} {
		got := filterHolds(t, env, tc.filter, doc)

		assert.Equal(t, tc.want, got, name)
	}
}

// A target that is no object holds no property: every test fails on it and a negation holds.
func TestFilterOnATargetThatIsNoObject(t *testing.T) {
	env := testhelper.Setup(t)
	set := prop("k", presence("set"))
	notSet := conditiontree.EventFilter{Group: groupOf(conditiontree.And, true, set)}

	for name, doc := range map[string]string{"JSON null": `null`, "a list": `[{"k": "Vip"}]`, "a text": `"k"`} {
		holds := filterHolds(t, env, filterOf(set), doc)
		negated := filterHolds(t, env, notSet, doc)

		assert.False(t, holds, name)
		assert.True(t, negated, name)
	}
}

// The keys and values a filter tests are the path's variables: the path holds neither.
func TestFilterKeysAndValuesAreVariablesOfThePath(t *testing.T) {
	filter := filterOf(
		prop("key_one", text("eq", "value_one")),
		prop("key_two", text("neq", "value_two")),
		prop("key_three", text("beginsWith", "value_three")),
		prop("key_four", number("gt", 4242)),
		prop("key_five", presence("set")),
	)

	path, err := condition.CompileEventFilter(filter)
	require.NoError(t, err)

	for _, spliced := range []string{"key_", "value_", "4242"} {
		assert.NotContains(t, path.Text, spliced)
	}
	assert.JSONEq(t, `{"v0":"key_one","v1":"value_one","v2":"key_two","v3":"value_two","v4":"key_three",
		"v5":"value_three","v6":"key_four","v7":4242,"v8":"key_five"}`, string(path.Vars))
}

// A key is data whatever it holds: quotes, a dot, the path's own syntax.
func TestFilterKeyIsMatchedLiterally(t *testing.T) {
	env := testhelper.Setup(t)
	for _, key := range []string{`it's`, `a "quoted" key`, `a.b`, `$`, `@.key == "x") || (true`, `\`} {
		doc, err := json.Marshal(map[string]string{key: "here"})
		require.NoError(t, err)

		hit := filterHolds(t, env, filterOf(prop(key, text("eq", "here"))), string(doc))
		miss := filterHolds(t, env, filterOf(prop(key, text("eq", "there"))), string(doc))

		assert.True(t, hit, key)
		assert.False(t, miss, key)
	}
}

// like_regex takes its pattern only as a literal, so contains writes its operand into the path:
// whatever the operand holds (a quote, a backslash, the path's syntax, a regular expression), it is
// one literal, matched as plain text.
func TestFilterContainsMatchesItsOperandLiterally(t *testing.T) {
	env := testhelper.Setup(t)
	for name, operand := range map[string]string{
		"a quote":             `say "hi"`,
		"a backslash":         `C:\dir`,
		"a dot":               `a.b`,
		"a star":              `a*b`,
		"a breakout":          `") || exists(@) || ("`,
		"a pattern":           `^.*$`,
		"a unicode character": "café <tag> & ü",
		"a line break":        "a\nb",
	} {
		holding, err := json.Marshal(map[string]string{"k": "prefix " + operand + " suffix"})
		require.NoError(t, err)
		lacking, err := json.Marshal(map[string]string{"k": "prefix suffix"})
		require.NoError(t, err)

		hit := filterHolds(t, env, filterOf(prop("k", text("contains", operand))), string(holding))
		miss := filterHolds(t, env, filterOf(prop("k", text("contains", operand))), string(lacking))

		assert.True(t, hit, name)
		assert.False(t, miss, name)
	}
}

// An empty operand is in every text, as on a Contact's attribute, and in nothing else.
func TestFilterContainsOfNothingHoldsForAnyText(t *testing.T) {
	env := testhelper.Setup(t)
	empty := filterOf(prop("k", text("contains", "")))

	text := filterHolds(t, env, empty, `{"k": "anything"}`)
	number := filterHolds(t, env, empty, `{"k": 1}`)

	assert.True(t, text)
	assert.False(t, number)
}

// An Event filter holds property tests of the one Event it is about and nothing else: a test of the
// Contact (an attribute, the birthday, a Custom field, a Tag, Telegram or SMS reachability) or of
// its history (an event count) has no value in its tree, and a property test that names a subject
// is a Rule's. Each refusal names its entry.
func TestAnEventFilterHoldsPropertyTestsOnly(t *testing.T) {
	for name, tc := range map[string]struct {
		entry leaf
		msg   string
	}{
		"a Contact attribute":         {attribute("email", text("eq", "x")), `unknown entry kind "attribute"`},
		"the birthday":                {birthday(withinLast(7)), `unknown entry kind "birthday"`},
		"a Custom field":              {customField("plan", text("eq", "pro")), `unknown entry kind "customField"`},
		"a Tag":                       {tagLeaf("vip", "has"), `unknown entry kind "tag"`},
		"Telegram reachability":       {telegram("has"), `unknown entry kind "telegram"`},
		"SMS reachability":            {sms("has"), `unknown entry kind "sms"`},
		"history":                     {eventLeaf("signup", "atLeast", 1, 0), `unknown entry kind "event"`},
		"the triggering Event's test": {property("tag", text("eq", "vip")), "a filter's property test names no subject"},
	} {
		_, err := condition.CompileEventFilter(filterOf(prop("ok", presence("set")), opsGroup(tc.entry)))

		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "entry 1: entry 0: "+tc.msg, name)
	}
}

// What a property test's type holds but its data does not, and a tree that is no filter.
func TestAnEventFilterRefusesWhatItCannotRead(t *testing.T) {
	var noEntries conditiontree.EventFilter
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and"}`), &noEntries))

	for name, tc := range map[string]struct {
		filter conditiontree.EventFilter
		msg    string
	}{
		"no key":                  {filterOf(prop("", text("eq", "x"))), "entry 0: property key is required"},
		"a moment test":           {filterOf(prop("at", withinLast(7))), `entry 0: unknown test kind "withinLast"`},
		"an unknown operator":     {filterOf(prop("k", text("like", "x"))), `entry 0: unknown text operator "like"`},
		"an unknown combinator":   {conditiontree.EventFilter{Group: groupOf("xor", false)}, `unknown combinator "xor"`},
		"a group without entries": {noEntries, "group entries are required"},
	} {
		_, err := condition.CompileEventFilter(tc.filter)

		require.ErrorContains(t, err, tc.msg, name)
	}
}
