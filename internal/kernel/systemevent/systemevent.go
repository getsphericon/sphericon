// Package systemevent names the System events (GLOSSARY: System event): the actions sphericon itself
// records as Events of a Workspace, generated from the contract's SystemEventAction enum
// (actions_gen.go, written by cmd/scopegen), and the properties their Events carry that the core
// reads by name. Each action is also the bus type of the event that records it; a customer's own
// action is never one of them. It is a leaf, so the ent schema builds the retention predicate of
// the Events with it.
package systemevent

// BounceKindProperty is the property of an email.bounced Event that says which kind of bounce it
// was, one of the two below. Only a hard bounce suppresses; a soft one, such as a full mailbox, is
// temporary. What each means for Suppression is the suppression module's rule.
const (
	BounceKindProperty = "bounceKind"
	BounceKindHard     = "hard"
	BounceKindSoft     = "soft"
)
