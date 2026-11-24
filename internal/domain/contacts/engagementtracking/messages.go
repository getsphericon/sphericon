package engagementtracking

import (
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// IsTracked is whether an Outbound message is tracked (ADR 0120): its render decided it goes out
// tracked and its tracking was not withdrawn since. It is the one definition every reader uses (the
// send path's retry, the recorder, an Order's credit, the rates, a Broadcast's figures, the export,
// the Winner, the Link identity token, a Branch's open test); TrackedMessages and
// UntrackedMessages are the same in SQL. A message no render decided (skipped before it, or not
// rendered yet) is not tracked. It lives with the consent, in the contacts context, so that the
// readers below Outbound send (an Order's credit) share it with the ones above.
func IsTracked(msg *ent.OutboundMessage) bool {
	return msg.Tracked != nil && *msg.Tracked && msg.TrackingWithdrawnAt == nil
}

// TrackedMessages is IsTracked as a predicate: the messages that went out tracked and whose
// tracking was not withdrawn.
func TrackedMessages() predicate.OutboundMessage {
	return outboundmessage.And(outboundmessage.Tracked(true), outboundmessage.TrackingWithdrawnAtIsNil())
}

// UntrackedMessages is TrackedMessages' complement: the messages no render decided, that went out
// untracked, or whose tracking was withdrawn since.
func UntrackedMessages() predicate.OutboundMessage {
	return outboundmessage.Or(
		outboundmessage.TrackedIsNil(), outboundmessage.Tracked(false), outboundmessage.TrackingWithdrawnAtNotNil(),
	)
}
