package events

import (
	"slices"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// MachineProperty is the key of the Event property that marks an open or click made by software
// (ADR 0053, GLOSSARY: Machine engagement). The Event's `machine` column holds the same fact for
// queries; the property is there for segment conditions.
const MachineProperty = "machine"

// ClickActions are the Events of a click on a link of a message, one per Channel that tracks them
// (ADR 0083): what a link report counts and what Attribution credits.
var ClickActions = []string{NameEmailClicked, NameTelegramClicked, NameSmsClicked}

// IsEngagement reports whether action is an open or a click, the Events a machine can make.
func IsEngagement(action string) bool {
	return action == NameEmailOpened || slices.Contains(ClickActions, action)
}

// HumanOnly narrows a query of Events to the ones not made by software, for every figure that
// counts engagement: a machine Event stays in the log and in no count.
func HumanOnly() predicate.Event { return event.Machine(false) }

// CapturedProperty is the key of the Event property that marks an email.* Event of a send
// through a sender that delivers nowhere outside (a Test inbox): captured, not delivered. The
// Event's `captured` column holds the same fact for queries.
const CapturedProperty = "captured"

// NotCaptured narrows a query of Events to the ones not about a captured send, for every figure
// of real deliverability: mail kept in a Test inbox reached no mailbox, so it moves no rate.
func NotCaptured() predicate.Event { return event.Captured(false) }
