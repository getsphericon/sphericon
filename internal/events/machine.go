package events

import (
	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent/event"
)

// MachineProperty is the key of the Event property that marks an open or click made by software
// (ADR 0053, GLOSSARY: Machine engagement).
const MachineProperty = "machine"

// IsMachine reports whether the properties of a stored Event mark it as made by software.
func IsMachine(props map[string]any) bool {
	machine, _ := props[MachineProperty].(bool)
	return machine
}

// IsEngagement reports whether action is an open or a click, the Events a machine can make.
func IsEngagement(action string) bool {
	return action == NameEmailOpened || action == NameEmailClicked
}

// HumanOnly narrows a query of Events (through Modify) to the ones not made by software, for every
// figure that counts engagement: a machine Event stays in the log and in no count.
func HumanOnly(sel *sql.Selector) {
	// raw sql: the flag is a key of the JSON properties that is absent on a human Event, and
	// NOT (x = true) would drop the rows where the key is missing.
	sel.Where(sql.ExprP("COALESCE(" + sel.C(event.FieldProperties) + "->>'" + MachineProperty + "', '') <> 'true'"))
}

// CapturedProperty is the key of the Event property that marks an email.* Event of a send
// through a sender that delivers nowhere outside (a Test inbox): captured, not delivered.
const CapturedProperty = "captured"

// NotCaptured narrows a query of Events (through Modify) to the ones not about a captured send, for
// every figure of real deliverability: mail kept in a Test inbox reached no mailbox, so it moves no rate.
func NotCaptured(sel *sql.Selector) {
	// raw sql: the flag is a key of the JSON properties that is absent on a real send, and
	// NOT (x = true) would drop the rows where the key is missing.
	sel.Where(sql.ExprP("COALESCE(" + sel.C(event.FieldProperties) + "->>'" + CapturedProperty + "', '') <> 'true'"))
}
