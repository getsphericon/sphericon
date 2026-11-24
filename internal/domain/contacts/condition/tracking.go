package condition

import (
	"fmt"
	"slices"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// engagementTrackingLeaf tests the Contact's Engagement tracking consent on channel (ADR 0120), as
// recorded: whether it reads as state, or, with no state, whether any is recorded; negated by a
// neq or a notSet test.
type engagementTrackingLeaf struct {
	channel channels.Channel
	state   *engagementtracking.State
	negated bool
}

func (engagementTrackingLeaf) isNode() {}

// decodeEngagementTracking reads an `engagementTracking` leaf: the Channel, and a text test that
// compares the consent with granted or denied by one of the contract's StateOperators, or a
// presence test.
func decodeEngagementTracking(e entry) (node, error) {
	channel, err := trackingChannelOf(e.Channel)
	if err != nil {
		return nil, err
	}
	t, err := decodeTest(e.Test, conditiontree.EngagementTrackingTests)
	if err != nil {
		return nil, err
	}
	switch t := t.(type) {
	case presenceTest:
		return engagementTrackingLeaf{channel: channel, negated: !t.set}, nil
	case textTest:
		// The contract's test of a consent is a text test of its own operators (StateOperator).
		op := conditiontree.StateOperator(t.op)
		if !slices.Contains(conditiontree.StateOperators, op) {
			return nil, fmt.Errorf("an engagement tracking consent is compared by one of %v, not %q", conditiontree.StateOperators, t.op)
		}
		state := engagementtracking.State(t.value)
		if err := engagementtrackingconsent.StateValidator(state); err != nil {
			return nil, fmt.Errorf("an engagement tracking consent is %s or %s, not %q", engagementtracking.Granted, engagementtracking.Denied, t.value)
		}
		return engagementTrackingLeaf{channel: channel, state: &state, negated: op == conditiontree.StateNeq}, nil
	}
	return nil, fmt.Errorf("unsupported test %T for an engagement tracking consent", t)
}

// trackingChannelOf is the Channel an engagementTracking leaf names, or a refusal of one outside
// the contract.
func trackingChannelOf(c conditiontree.TrackingChannel) (channels.Channel, error) {
	switch c {
	case conditiontree.TrackingEmail:
		return channels.Email, nil
	case conditiontree.TrackingSms:
		return channels.SMS, nil
	case conditiontree.TrackingTelegram:
		return channels.Telegram, nil
	}
	return "", fmt.Errorf("unknown engagement tracking channel %q", c)
}

// contact is the leaf as a predicate, read live from the consent module's own SQL form. It is never
// NULL, so its negation keeps exactly the Contacts it leaves out: neq denied holds for a Contact
// with no consent recorded, or with no Destination on the Channel.
func (l engagementTrackingLeaf) contact() predicate.Contact {
	var p predicate.Contact
	if l.state != nil {
		p = engagementtracking.ReadsWhere(l.channel, *l.state)
	} else {
		p = contact.Or(
			engagementtracking.ReadsWhere(l.channel, engagementtracking.Granted),
			engagementtracking.ReadsWhere(l.channel, engagementtracking.Denied),
		)
	}
	if l.negated {
		return contact.Not(p)
	}
	return p
}
