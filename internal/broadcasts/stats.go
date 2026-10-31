package broadcasts

import (
	"context"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/aggregates"
	"github.com/getsphericon/sphericon/internal/orders"
)

// Stats is a Broadcast's delivery figures. They are never stored: the audience is its
// recipient rows, how each send ended is its Outbound message (ADR 0015), and engagement is
// the timestamps on the recipient, so the figures cannot drift from what the rows say and are
// live while the Broadcast is still sending.
type Stats struct {
	// Recipients counts the Broadcast's recipient rows.
	Recipients int
	// Sent, Skipped and Failed count the recipients whose message ended that way.
	Sent, Skipped, Failed int
	// Pending counts the recipients whose send has not ended: no message yet, or one still
	// claimed. It is Recipients less the three above.
	Pending int
	// Opened, Clicked and Unsubscribed count the recipients with that engagement.
	Opened, Clicked, Unsubscribed int
	// Delivered, Bounced and Complained count the messages whose delivery result is stamped
	// (ADR 0053). They are read from the stamps, never from Events, so they outlive retention.
	Delivered, Bounced, Complained int
	// Captured counts the messages that went through a sender delivering nowhere outside (a
	// Test inbox): captured, not delivered. They are stamped delivered too, so a reader
	// can tell them apart from a real delivery.
	Captured int
	// Revenue is the Orders credited to the Broadcast and their net revenue, in the Reporting
	// currency (ADR 0064).
	Revenue orders.Revenue
}

// Processed counts the recipients whose send has ended.
func (st Stats) Processed() int { return st.Recipients - st.Pending }

// OpenRate is the share of sent messages that were opened, in [0,1].
func (st Stats) OpenRate() float32 { return Ratio(st.Opened, st.Sent) }

// ClickRate is the share of sent messages that were clicked, in [0,1].
func (st Stats) ClickRate() float32 { return Ratio(st.Clicked, st.Sent) }

// DeliveryRate is the share of recipients whose message was sent, in [0,1].
func (st Stats) DeliveryRate() float32 { return Ratio(st.Sent, st.Recipients) }

// ClickToOpenRate is the share of opened messages that were clicked, in [0,1].
func (st Stats) ClickToOpenRate() float32 { return Ratio(st.Clicked, st.Opened) }

// UnsubscribeRate is the share of sent messages whose recipient unsubscribed, in [0,1].
func (st Stats) UnsubscribeRate() float32 { return Ratio(st.Unsubscribed, st.Sent) }

// FailureRate is the share of recipients whose send failed, in [0,1].
func (st Stats) FailureRate() float32 { return Ratio(st.Failed, st.Recipients) }

// Ratio is num/denom as a float32 in [0,1]; a zero denominator yields 0.
func Ratio(num, denom int) float32 {
	if denom <= 0 {
		return 0
	}
	return float32(num) / float32(denom)
}

// Pending matches the recipients whose send has not ended: no Outbound message of theirs has
// left pending. It is the one reading of "this recipient is still to be sent".
func Pending() predicate.BroadcastRecipient {
	return func(sel *sql.Selector) {
		m := sql.Table(outboundmessage.Table).As("ended")
		sel.Where(sql.NotExists(
			sql.Select(m.C(outboundmessage.FieldID)).From(m).Where(sql.And(
				sql.ColumnsEQ(m.C(outboundmessage.FieldBroadcastRecipientID), sel.C(broadcastrecipient.FieldID)),
				sql.NEQ(m.C(outboundmessage.FieldStatus), string(outboundmessage.StatusPending)),
			)),
		))
	}
}

// StatsOf reads the figures of the given Broadcasts in two grouped queries, whatever their
// number. A Broadcast with no rows yet has zero figures.
func StatsOf(ctx context.Context, s *ent.Scoped, ids ...int64) (map[int64]Stats, error) {
	out := make(map[int64]Stats, len(ids))
	for _, id := range ids {
		out[id] = Stats{}
	}
	if len(ids) == 0 {
		return out, nil
	}

	var recipients []struct {
		BroadcastID  int64 `json:"broadcast_id"`
		Total        int   `json:"total"`
		Opened       int   `json:"opened"`
		Clicked      int   `json:"clicked"`
		Unsubscribed int   `json:"unsubscribed"`
	}
	if err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastIDIn(ids...)).
		GroupBy(broadcastrecipient.FieldBroadcastID).
		Aggregate(
			ent.As(ent.Count(), "total"),
			aggregates.CountSet(broadcastrecipient.FieldOpenedAt, "opened"),
			aggregates.CountSet(broadcastrecipient.FieldClickedAt, "clicked"),
			aggregates.CountSet(broadcastrecipient.FieldUnsubscribedAt, "unsubscribed"),
		).
		Scan(ctx, &recipients); err != nil {
		return nil, err
	}
	for _, r := range recipients {
		st := out[r.BroadcastID]
		st.Recipients, st.Opened, st.Clicked, st.Unsubscribed = r.Total, r.Opened, r.Clicked, r.Unsubscribed
		out[r.BroadcastID] = st
	}

	var messages []struct {
		BroadcastID int64                  `json:"broadcast_id"`
		Status      outboundmessage.Status `json:"status"`
		Count       int                    `json:"count"`
		Delivered   int                    `json:"delivered"`
		Bounced     int                    `json:"bounced"`
		Complained  int                    `json:"complained"`
		Captured    int                    `json:"captured"`
	}
	if err := s.OutboundMessage().Query().
		Where(outboundmessage.BroadcastIDIn(ids...)).
		GroupBy(outboundmessage.FieldBroadcastID, outboundmessage.FieldStatus).
		Aggregate(
			ent.As(ent.Count(), "count"),
			aggregates.CountSet(outboundmessage.FieldDeliveredAt, "delivered"),
			aggregates.CountSet(outboundmessage.FieldBouncedAt, "bounced"),
			aggregates.CountSet(outboundmessage.FieldComplainedAt, "complained"),
			aggregates.CountTrue(outboundmessage.FieldCaptured, "captured"),
		).
		Scan(ctx, &messages); err != nil {
		return nil, err
	}
	for _, m := range messages {
		st := out[m.BroadcastID]
		st.Delivered += m.Delivered
		st.Bounced += m.Bounced
		st.Complained += m.Complained
		st.Captured += m.Captured
		switch m.Status {
		case outboundmessage.StatusSent:
			st.Sent = m.Count
		case outboundmessage.StatusSkipped:
			st.Skipped = m.Count
		case outboundmessage.StatusFailed:
			st.Failed = m.Count
		case outboundmessage.StatusPending:
			// still claimed: not ended, so it stays in Pending
		}
		out[m.BroadcastID] = st
	}
	revenue, err := orders.CreditedToBroadcasts(ctx, s, ids...)
	if err != nil {
		return nil, err
	}
	for id, st := range out {
		st.Pending = max(0, st.Recipients-st.Sent-st.Skipped-st.Failed)
		st.Revenue = revenue.Of(id)
		out[id] = st
	}
	return out, nil
}
