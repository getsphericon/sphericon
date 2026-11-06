package broadcasts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// The unsubscribe of a recipient is set once, by a conditional update, and reported as whether
// this call set it, so a repeated click on the link is counted exactly once. Its opens and clicks
// are the Outbound message's stamps (outboundmessages.StampOpened, ADR 0053).

// RecordUnsubscribe marks the Contact's recipient row of the Broadcast as having unsubscribed
// from it. A Contact that was never a recipient of it (a link to an erased row) matches nothing.
func RecordUnsubscribe(ctx context.Context, s *ent.Scoped, broadcastID, contactID int64) (bool, error) {
	return setOnce(ctx, s,
		broadcastrecipient.And(broadcastrecipient.BroadcastID(broadcastID), broadcastrecipient.ContactID(contactID)),
		broadcastrecipient.UnsubscribedAtIsNil(),
		func(u *ent.BroadcastRecipientScopedUpdate, at time.Time) { u.SetUnsubscribedAt(at) })
}

func setOnce(ctx context.Context, s *ent.Scoped, which, unset predicate.BroadcastRecipient, set func(*ent.BroadcastRecipientScopedUpdate, time.Time)) (bool, error) {
	u := s.BroadcastRecipient().Update().Where(which, unset)
	set(u, time.Now())
	n, err := u.Save(ctx)
	return n > 0, err
}
