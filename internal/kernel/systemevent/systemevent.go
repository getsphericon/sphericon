// Package systemevent names the System events (GLOSSARY: System event): the actions sphericon itself
// records as Events of a Workspace, generated from the contract's SystemEventAction enum
// (actions_gen.go, written by cmd/scopegen), and the properties their Events carry that the core
// reads by name. Each action is also the bus type of the event that records it; a customer's own
// action is never one of them. It is a leaf, so the ent schema builds the retention predicate of
// the Events with it.
package systemevent

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"
)

// BounceKindProperty is the property of an email.bounced Event that says which kind of bounce it
// was, one of the two below. Only a hard bounce suppresses; a soft one, such as a full mailbox, is
// temporary. What each means for Suppression is the suppression module's rule.
const (
	BounceKindProperty = "bounceKind"
	BounceKindHard     = "hard"
	BounceKindSoft     = "soft"
)

// KeyProperties are the properties of a System event that hold a primary key: what the Event is
// about (an Automation, a Broadcast, an Integration), the message it came from, who asked.
var KeyProperties = []string{"automationId", "broadcastId", "integrationId", "outboundMessageId", "requesterId"}

// PublicProperties is the properties of an Event of action as a client sees them (ADR 0142): a
// System event's KeyProperties by their public form (publicid). A customer's own Event is its
// own data, whatever its keys are called, and stays as it was.
func PublicProperties(action string, props map[string]any) map[string]any {
	if props == nil || !slices.Contains(Actions, action) {
		return props
	}
	out := maps.Clone(props)
	for _, key := range KeyProperties {
		if id, ok := keyOf(out[key]); ok {
			out[key] = publicid.Encode(id)
		}
	}
	return out
}

// keyOf is a stored key property, a JSON number however it was decoded.
func keyOf(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), n == float64(int64(n))
	case json.Number:
		id, err := n.Int64()
		return id, err == nil
	default:
		return 0, false
	}
}
