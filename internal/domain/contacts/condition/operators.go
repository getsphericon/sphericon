package condition

import (
	"encoding/json"
	"strings"

	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// operator is how a leaf or a test compares: the operators of the typed tests and of the Tag,
// Telegram and SMS leaves.
type operator string

const (
	opEQ         operator = "eq"
	opNEQ        operator = "neq"
	opContains   operator = "contains"
	opBeginsWith operator = "beginsWith"

	opSet    operator = "set"
	opNotSet operator = "notSet"

	opHas         operator = "has"
	opDoesNotHave operator = "doesNotHave"

	opGT  operator = "gt"
	opGTE operator = "gte"
	opLT  operator = "lt"
	opLTE operator = "lte"

	opBefore operator = "before"
	opAfter  operator = "after"
	// opWithin is the moment test of the last days up to now, the `withinLast` test.
	opWithin operator = "within"
)

// textTest's operators, in the two places a Condition is evaluated: contact compiles one to a
// predicate on a Contact attribute (SQL), event runs it in Go on a property of the triggering
// Event. They are one table so a Contact leaf and a Rule's test of the triggering Event mean the
// same by the same operator; TestOperatorsAgreeOnTheContactSideAndTheEventSide runs the same
// cases through both.
type textOperator struct {
	contact func(f textField, want string) predicate.Contact
	// event reports whether the property's value, present in the Event, passes. Only a string
	// can: a value of another type fails every operator.
	event func(have any, want string) bool
}

// text returns the property as a string, false for any other type (JSON null included).
func text(have any) (string, bool) {
	s, ok := have.(string)
	return s, ok
}

var textOperators = map[operator]textOperator{
	opEQ: {
		contact: func(f textField, want string) predicate.Contact { return f.eq(want) },
		event:   func(have any, want string) bool { s, ok := text(have); return ok && s == want },
	},
	opNEQ: {
		contact: func(f textField, want string) predicate.Contact { return f.neq(want) },
		event:   func(have any, want string) bool { s, ok := text(have); return ok && s != want },
	},
	opContains: {
		contact: func(f textField, want string) predicate.Contact { return f.contains(want) },
		event: func(have any, want string) bool {
			s, ok := text(have)
			return ok && strings.Contains(strings.ToLower(s), strings.ToLower(want))
		},
	},
	opBeginsWith: {
		contact: func(f textField, want string) predicate.Contact { return f.beginsWith(want) },
		event:   func(have any, want string) bool { s, ok := text(have); return ok && strings.HasPrefix(s, want) },
	},
}

// contact is the presence test of a Contact's text attribute: set or null. Like textOperators, it
// means the same as event, its test of a property of the triggering Event.
func (p presenceTest) contact(f textField) predicate.Contact {
	if p.set {
		return f.notNull
	}
	return f.null
}

// event is the presence test of a property present in the triggering Event: set is a text value,
// not set is JSON null.
func (p presenceTest) event(have any) bool {
	if p.set {
		_, ok := text(have)
		return ok
	}
	return have == nil
}

// operator is the operator of a value's presence test, set or notSet: the one the table of what
// fits a Custom field (customFieldTests) is written in.
func (p presenceTest) operator() operator {
	if p.set {
		return opSet
	}
	return opNotSet
}

// comparison is one operator of a number test, in the two places it is evaluated: filter is its
// jsonpath test of a number Custom field's value @ against the operand $v (SQL), holds runs it in
// Go on a number property of the triggering Event. One table, so a Segment's leaf and a Rule's
// test of the triggering Event mean the same by the same operator;
// TestNumberLeavesAgreeOnTheContactSideAndTheEventSide runs the same cases through both.
type comparison struct {
	filter string
	holds  func(have, want float64) bool
}

var comparisons = map[operator]comparison{
	opGT:  {filter: "@ > $v", holds: func(have, want float64) bool { return have > want }},
	opGTE: {filter: "@ >= $v", holds: func(have, want float64) bool { return have >= want }},
	opLT:  {filter: "@ < $v", holds: func(have, want float64) bool { return have < want }},
	opLTE: {filter: "@ <= $v", holds: func(have, want float64) bool { return have <= want }},
}

// number returns the property as a number, false for any other type: encoding/json decodes a
// JSON number to float64, or to json.Number when the decoder keeps numbers (UseNumber), as
// collection does.
func number(have any) (float64, bool) {
	switch v := have.(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// A datetime Custom field's value @ is a moment in one of two kinds, and each is compared only with
// a bound of its own kind: jsonpath refuses to compare a date or a date-time without an offset with
// one that has an offset (outside the _tz functions, which would read the session's zone), and
// silent mode does not hide that error. && stops at the kind test, so the comparison never meets
// the other kind; a value of neither kind (a time alone, text that is no date) matches no side.
const (
	// isZoned is a date-time with an offset, compared with a bound written as a moment.
	isZoned = `@.datetime().type() == "timestamp with time zone"`
	// isLocal is a date-time without an offset or a bare date (the start of that day): read in the
	// Workspace time zone (ADR 0087), so it is compared with the bound written as the wall-clock time
	// there. In the hour a daylight-saving change repeats, a value names the first of the two.
	isLocal = `(@.datetime().type() == "timestamp without time zone" || @.datetime().type() == "date")`
)

// momentLayout writes a bound of a moment test as jsonpath's datetime() reads a moment: RFC 3339
// with the offset written out, never "Z".
const momentLayout = "2006-01-02T15:04:05.999999-07:00"

// localLayout writes a bound of a moment test as the wall-clock time in the Workspace time zone,
// without an offset, which jsonpath's datetime() reads as a timestamp without a time zone.
const localLayout = "2006-01-02T15:04:05.999999"

// eitherKind is the jsonpath filter that holds when @ is a moment with an offset and zoned holds,
// or a moment without one and local holds: the same test, against the bounds of each kind.
func eitherKind(zoned, local string) string {
	return "(" + isZoned + " && " + zoned + ") || (" + isLocal + " && " + local + ")"
}

// sides is each operator of a moment test as its jsonpath filter of a datetime Custom field against
// the bound $at ($atLocal for a value without an offset): before is strictly earlier, after is at
// or later, so the two split time at the bound.
var sides = map[operator]string{
	opBefore: eitherKind("@.datetime() < $at.datetime()", "@.datetime() < $atLocal.datetime()"),
	opAfter:  eitherKind("@.datetime() >= $at.datetime()", "@.datetime() >= $atLocal.datetime()"),
}

// window is the jsonpath filter of a withinLast test: a moment from $since up to $now, each bound
// written in both kinds.
var window = eitherKind(
	"@.datetime() >= $since.datetime() && @.datetime() <= $now.datetime()",
	"@.datetime() >= $sinceLocal.datetime() && @.datetime() <= $nowLocal.datetime()",
)

// fieldTests is what a Custom field leaf may ask of a field of one type: per typed test, the
// operators it takes (none: the type does not take the test), and whether it takes withinLast.
type fieldTests struct {
	text, number, moment, presence []operator
	withinLast                     bool
}

// customFieldTests is what a Custom field leaf may ask of a field, by its type: the one table of
// what fits, which the API lists as each Custom field's conditionTests. A text test's contains is
// the membership of a JSON array, read from a string field; a number or boolean field's eq and neq
// compare the text of its value. No type takes notSet: a field not set is no test of its type.
var customFieldTests = map[customfield.Type]fieldTests{
	customfield.TypeString:   {text: []operator{opEQ, opNEQ, opContains}, presence: []operator{opSet}},
	customfield.TypeBool:     {text: []operator{opEQ, opNEQ}, presence: []operator{opSet}},
	customfield.TypeNumber:   {text: []operator{opEQ, opNEQ}, number: []operator{opGT, opGTE, opLT, opLTE}, presence: []operator{opSet}},
	customfield.TypeDatetime: {moment: []operator{opBefore, opAfter}, withinLast: true, presence: []operator{opSet}},
}

// Tests is what a Condition may ask of a Custom field of one type: per typed test, the operators
// it takes there (none: the type does not take the test), and whether it takes withinLast.
type Tests struct {
	Text, Number, Moment, Presence []string
	WithinLast                     bool
}

// CustomFieldTests is what a Condition may ask of a Custom field of type t.
func CustomFieldTests(t customfield.Type) Tests {
	fits := customFieldTests[t]
	return Tests{
		Text:       names(fits.text),
		Number:     names(fits.number),
		Moment:     names(fits.moment),
		Presence:   names(fits.presence),
		WithinLast: fits.withinLast,
	}
}

// names is each operator's name.
func names(ops []operator) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = string(op)
	}
	return out
}
