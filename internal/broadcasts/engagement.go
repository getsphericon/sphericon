package broadcasts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// The engagement of a recipient with the email it was sent. Each is set once, by a conditional
// update, and reports whether this call set it, so a repeated hit (a mailbox that fetches the
// pixel twice, a double click) is counted exactly once. The caller publishes the engagement
// event only for the call that set it, in the same transaction.

// RecordOpen marks the recipient as having opened the email.
func RecordOpen(ctx context.Context, s *ent.Scoped, recipientID int64) (bool, error) {
	return setOnce(ctx, s, broadcastrecipient.ID(recipientID), broadcastrecipient.OpenedAtIsNil(),
		func(u *ent.BroadcastRecipientScopedUpdate, at time.Time) { u.SetOpenedAt(at) })
}

// RecordClick marks the recipient as having clicked a link of the email.
func RecordClick(ctx context.Context, s *ent.Scoped, recipientID int64) (bool, error) {
	return setOnce(ctx, s, broadcastrecipient.ID(recipientID), broadcastrecipient.ClickedAtIsNil(),
		func(u *ent.BroadcastRecipientScopedUpdate, at time.Time) { u.SetClickedAt(at) })
}

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
