package condition

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// The meaning of each operator of the contract (conditiontree), decided by a switch over its
// generated values: a value the contract adds and nothing here means fails the exhaustive lint,
// and a value outside the contract is refused.

// textOperator is what one operator of a text test means in the two places a Condition is
// evaluated: contact compiles it to a predicate on a Contact attribute (SQL), event runs it in Go
// on a property of the triggering Event. They are one value so a Contact leaf and a Rule's test of
// the triggering Event mean the same by the same operator;
// TestOperatorsAgreeOnTheContactSideAndTheEventSide runs the same cases through both.
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

// textOperatorOf is what op means, or a refusal of an operator outside the contract.
func textOperatorOf(op conditiontree.TextOperator) (textOperator, error) {
	switch op {
	case conditiontree.TextEq:
		return textOperator{
			contact: func(f textField, want string) predicate.Contact { return f.eq(want) },
			event:   func(have any, want string) bool { s, ok := text(have); return ok && s == want },
		}, nil
	case conditiontree.TextNeq:
		return textOperator{
			contact: func(f textField, want string) predicate.Contact { return f.neq(want) },
			event:   func(have any, want string) bool { s, ok := text(have); return ok && s != want },
		}, nil
	case conditiontree.TextContains:
		return textOperator{
			contact: func(f textField, want string) predicate.Contact { return f.contains(want) },
			event: func(have any, want string) bool {
				s, ok := text(have)
				return ok && strings.Contains(strings.ToLower(s), strings.ToLower(want))
			},
		}, nil
	case conditiontree.TextBeginsWith:
		return textOperator{
			contact: func(f textField, want string) predicate.Contact { return f.beginsWith(want) },
			event:   func(have any, want string) bool { s, ok := text(have); return ok && strings.HasPrefix(s, want) },
		}, nil
	}
	return textOperator{}, fmt.Errorf("unknown text operator %q", op)
}

// contact is the presence test of a Contact's text attribute: set or null. Like textOperator, it
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
func (p presenceTest) operator() conditiontree.PresenceOperator {
	if p.set {
		return conditiontree.PresenceSet
	}
	return conditiontree.PresenceNotSet
}

// comparison is what one operator of a number test means in the two places it is evaluated: filter
// is its jsonpath test of a number Custom field's value @ against the operand $v (SQL), holds runs
// it in Go on a number property of the triggering Event. One value, so a Segment's leaf and a Rule's
// test of the triggering Event mean the same by the same operator;
// TestNumberLeavesAgreeOnTheContactSideAndTheEventSide runs the same cases through both.
type comparison struct {
	filter string
	holds  func(have, want float64) bool
}

// comparisonOf is what op means, or a refusal of an operator outside the contract.
func comparisonOf(op conditiontree.NumberOperator) (comparison, error) {
	switch op {
	case conditiontree.NumberGt:
		return comparison{filter: "@ > $v", holds: func(have, want float64) bool { return have > want }}, nil
	case conditiontree.NumberGte:
		return comparison{filter: "@ >= $v", holds: func(have, want float64) bool { return have >= want }}, nil
	case conditiontree.NumberLt:
		return comparison{filter: "@ < $v", holds: func(have, want float64) bool { return have < want }}, nil
	case conditiontree.NumberLte:
		return comparison{filter: "@ <= $v", holds: func(have, want float64) bool { return have <= want }}, nil
	}
	return comparison{}, fmt.Errorf("unknown number operator %q", op)
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

// side is what one operator of a moment test means: before is strictly earlier than the bound,
// after is at or later, so the two split time at it. filter is its jsonpath test of a datetime
// Custom field against the bound $at ($atLocal for a value without an offset), birthday its
// predicate on the Contact's date of birth.
type side struct {
	filter   string
	birthday func(time.Time) predicate.Contact
}

// sideOf is what op means, or a refusal of an operator outside the contract.
func sideOf(op conditiontree.MomentOperator) (side, error) {
	switch op {
	case conditiontree.MomentBefore:
		return side{
			filter:   eitherKind("@.datetime() < $at.datetime()", "@.datetime() < $atLocal.datetime()"),
			birthday: contact.BirthdayLT,
		}, nil
	case conditiontree.MomentAfter:
		return side{
			filter:   eitherKind("@.datetime() >= $at.datetime()", "@.datetime() >= $atLocal.datetime()"),
			birthday: contact.BirthdayGTE,
		}, nil
	}
	return side{}, fmt.Errorf("unknown moment operator %q", op)
}

// window is the jsonpath filter of a withinLast test: a moment from $since up to $now, each bound
// written in both kinds.
var window = eitherKind(
	"@.datetime() >= $since.datetime() && @.datetime() <= $now.datetime()",
	"@.datetime() >= $sinceLocal.datetime() && @.datetime() <= $nowLocal.datetime()",
)

// Tests is what a Condition may ask of a Custom field of one type: per typed test, the operators
// it takes there (none: the type does not take the test), and whether it takes withinLast.
type Tests struct {
	Text       []conditiontree.TextOperator
	Number     []conditiontree.NumberOperator
	Moment     []conditiontree.MomentOperator
	Presence   []conditiontree.PresenceOperator
	WithinLast bool
}

// customFieldTests is what a Custom field leaf may ask of a field, by its type: the one table of
// what fits, which the API lists as each Custom field's conditionTests. A text test's contains is
// the membership of a JSON array, read from a string field; a number or boolean field's eq and neq
// compare the text of its value. No type takes notSet: a field not set is no test of its type.
var customFieldTests = map[customfield.Type]Tests{
	customfield.TypeString: {
		Text:     []conditiontree.TextOperator{conditiontree.TextEq, conditiontree.TextNeq, conditiontree.TextContains},
		Presence: []conditiontree.PresenceOperator{conditiontree.PresenceSet},
	},
	customfield.TypeBool: {
		Text:     []conditiontree.TextOperator{conditiontree.TextEq, conditiontree.TextNeq},
		Presence: []conditiontree.PresenceOperator{conditiontree.PresenceSet},
	},
	customfield.TypeNumber: {
		Text:     []conditiontree.TextOperator{conditiontree.TextEq, conditiontree.TextNeq},
		Number:   conditiontree.NumberOperators,
		Presence: []conditiontree.PresenceOperator{conditiontree.PresenceSet},
	},
	customfield.TypeDatetime: {
		Moment:     conditiontree.MomentOperators,
		WithinLast: true,
		Presence:   []conditiontree.PresenceOperator{conditiontree.PresenceSet},
	},
}

// CustomFieldTests is what a Condition may ask of a Custom field of type t.
func CustomFieldTests(t customfield.Type) Tests {
	return customFieldTests[t]
}
