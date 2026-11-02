package condition

import (
	"encoding/json"
	"strings"

	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// operator is a test a leaf names: the operators of the rule, number and date leaves.
type operator string

const (
	opEQ          operator = "eq"
	opNEQ         operator = "neq"
	opContains    operator = "contains"
	opBeginsWith  operator = "beginsWith"
	opNull        operator = "null"
	opNotNull     operator = "notNull"
	opHas         operator = "has"
	opDoesNotHave operator = "doesNotHave"

	opGT  operator = "gt"
	opGTE operator = "gte"
	opLT  operator = "lt"
	opLTE operator = "lte"

	opBefore operator = "before"
	opAfter  operator = "after"
)

// textTest is one test of a text value, in the two places a Condition is evaluated: contact
// compiles it to a predicate on a Contact attribute (SQL), event runs it in Go on a property of
// the triggering Event. They are one table so a Contact leaf and a Rule's "trigger:" leaf mean
// the same by the same operator; TestOperatorsAgreeOnTheContactSideAndTheEventSide runs the same
// cases through both.
type textTest struct {
	contact func(f textField, want string) predicate.Contact
	// event reports whether the property's value, present in the Event, passes. Only a string
	// can: a value of another type fails every operator but null.
	event func(have any, want string) bool
}

// text returns the property as a string, false for any other type (JSON null included).
func text(have any) (string, bool) {
	s, ok := have.(string)
	return s, ok
}

var textTests = map[operator]textTest{
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
	opNull: {
		contact: func(f textField, _ string) predicate.Contact { return f.null },
		event:   func(have any, _ string) bool { return have == nil },
	},
	opNotNull: {
		contact: func(f textField, _ string) predicate.Contact { return f.notNull },
		event:   func(have any, _ string) bool { _, ok := text(have); return ok },
	},
}

// comparison is one operator of a number leaf, in the two places it is evaluated: filter is its
// jsonpath test of a number Custom field's value @ against the operand $v (SQL), holds runs it in
// Go on a number property of the triggering Event. One table, so a Segment's leaf and a Rule's
// "trigger:" leaf mean the same by the same operator;
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

// momentLayout writes a bound of a date leaf as jsonpath's datetime() reads a moment: RFC 3339
// with the offset written out, never "Z".
const momentLayout = "2006-01-02T15:04:05.999999-07:00"

// localLayout writes a bound of a date leaf as the wall-clock time in the Workspace time zone,
// without an offset, which jsonpath's datetime() reads as a timestamp without a time zone.
const localLayout = "2006-01-02T15:04:05.999999"

// eitherKind is the jsonpath filter that holds when @ is a moment with an offset and zoned holds,
// or a moment without one and local holds: the same test, against the bounds of each kind.
func eitherKind(zoned, local string) string {
	return "(" + isZoned + " && " + zoned + ") || (" + isLocal + " && " + local + ")"
}

// sides is each operator of a date leaf as its jsonpath filter of a datetime Custom field against
// the bound $at ($atLocal for a value without an offset): before is strictly earlier, after is at
// or later, so the two split time at the bound.
var sides = map[operator]string{
	opBefore: eitherKind("@.datetime() < $at.datetime()", "@.datetime() < $atLocal.datetime()"),
	opAfter:  eitherKind("@.datetime() >= $at.datetime()", "@.datetime() >= $atLocal.datetime()"),
}

// window is the jsonpath filter of a date-within leaf: a moment from $since up to $now, each bound
// written in both kinds.
var window = eitherKind(
	"@.datetime() >= $since.datetime() && @.datetime() <= $now.datetime()",
	"@.datetime() >= $sinceLocal.datetime() && @.datetime() <= $nowLocal.datetime()",
)

// customFieldOperators is what a rule leaf may test on a Custom field, by the field's type: the one
// table of what fits, which the API lists as each Custom field's ruleOperators. contains is the
// membership of a JSON array, read from a string field. A number field is also compared by a
// number leaf, a datetime field tested by the date leaves.
var customFieldOperators = map[customfield.Type][]operator{
	customfield.TypeString:   {opEQ, opNEQ, opContains, opNotNull},
	customfield.TypeBool:     {opEQ, opNEQ, opNotNull},
	customfield.TypeNumber:   {opEQ, opNEQ, opNotNull},
	customfield.TypeDatetime: {opNotNull},
}

// RuleOperators is the operators a rule leaf may test on a Custom field of type t.
func RuleOperators(t customfield.Type) []string {
	ops := customFieldOperators[t]
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = string(op)
	}
	return out
}
