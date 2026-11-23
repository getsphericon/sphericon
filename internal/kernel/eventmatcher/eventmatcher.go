// Package eventmatcher is an Event matcher as an Automation version stores it (ADR 0133): an
// action and an optional filter on that Event's own properties. A version's Trigger of kind
// `event` is one, and so are its exit events and its goal event.
package eventmatcher

import "github.com/getsphericon/sphericon/internal/kernel/conditiontree"

// Matcher names the Events it matches: those of Action whose properties pass Filter. A nil Filter
// matches every Event of the action. Its JSON form is the document a matcher column holds.
type Matcher struct {
	Action string                     `json:"action"`
	Filter *conditiontree.EventFilter `json:"filter,omitempty"`
}
