package condition

import (
	"strings"

	"github.com/getsphericon/sphericon/ent/predicate"
)

// operator is one test of a text value, in the two places a Condition is evaluated: contact
// compiles it to a predicate on a Contact attribute (SQL), event runs it in Go on a property of
// the triggering Event. They are one table so a Contact leaf and a Rule's "trigger:" leaf mean
// the same by the same operator; operators_parity_test.go runs the same cases through both.
type operator struct {
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

// contract-copy: the one backend table of the text operators, read by both sides, not a second list
var operators = map[string]operator{
	"eq": {
		contact: func(f textField, want string) predicate.Contact { return f.eq(want) },
		event:   func(have any, want string) bool { s, ok := text(have); return ok && s == want },
	},
	"neq": {
		contact: func(f textField, want string) predicate.Contact { return f.neq(want) },
		event:   func(have any, want string) bool { s, ok := text(have); return ok && s != want },
	},
	"contains": {
		contact: func(f textField, want string) predicate.Contact { return f.contains(want) },
		event: func(have any, want string) bool {
			s, ok := text(have)
			return ok && strings.Contains(strings.ToLower(s), strings.ToLower(want))
		},
	},
	"beginsWith": {
		contact: func(f textField, want string) predicate.Contact { return f.beginsWith(want) },
		event:   func(have any, want string) bool { s, ok := text(have); return ok && strings.HasPrefix(s, want) },
	},
	"null": {
		contact: func(f textField, _ string) predicate.Contact { return f.null },
		event:   func(have any, _ string) bool { return have == nil },
	},
	"notNull": {
		contact: func(f textField, _ string) predicate.Contact { return f.notNull },
		event:   func(have any, _ string) bool { _, ok := text(have); return ok },
	},
}
