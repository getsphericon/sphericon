package broadcasts

import (
	"context"
	"slices"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastfigure"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/infra/aggregates"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Stats is a Broadcast's delivery figures. They are read from its rows: the audience is its
// recipient rows, how each send ended is its Outbound message (ADR 0015), and engagement is the
// stamps on the message, so the figures cannot drift from what the rows say and are live while the
// Broadcast is still sending. Once message retention freezes a finished Broadcast (ADR 0143), they
// are its BroadcastFigure, written once before its rows go.
type Stats struct {
	// Channel is the Channel of the Broadcast: it decides which figures are reported (the Reports
	// methods). StatsOf leaves it empty; StatsFor sets it.
	Channel channels.Channel
	// Recipients counts the Broadcast's recipient rows.
	Recipients int
	// Sent, Skipped and Failed count the recipients whose message ended that way.
	Sent, Skipped, Failed int
	// Pending counts the recipients whose send has not ended: no message yet, or one still
	// claimed. It is Recipients less the three above.
	Pending int
	// Opened, Clicked and Unsubscribed count the recipients with that engagement.
	Opened, Clicked, Unsubscribed int
	// Delivered, Undelivered, Bounced and Complained count the messages whose delivery result is
	// stamped (ADR 0053, ADR 0079). They are read from the stamps, never from Events, so they
	// outlive retention.
	Delivered, Undelivered, Bounced, Complained int
	// Captured counts the messages that went through a sender delivering nowhere outside (a
	// Test inbox): captured, not delivered. They are stamped delivered too, so a reader
	// can tell them apart from a real delivery.
	Captured int
	// Revenue is the Orders credited to the Broadcast and their net revenue, in the Reporting
	// currency (ADR 0064).
	Revenue orders.Revenue
}

// Figures is the Stats as the Broadcast's Channel reports them (ADR 0080): every Channel reports
// recipients, sent, skipped, failed, pending, clicked and unsubscribed; opened, delivered,
// undelivered, bounced, complained and captured, and the rates built on them, are nil on a Channel
// that does not report them, never zero. Telegram and SMS have no delivery rate and no open rate.
type Figures struct {
	Recipients, Sent, Skipped, Failed, Pending, Clicked, Unsubscribed int

	Opened, Delivered, Undelivered, Bounced, Complained, Captured *int

	DeliveryRate, OpenRate, ClickToOpenRate *float32
	ClickRate, UnsubscribeRate, FailureRate float32

	Revenue orders.Revenue
}

// Figures reads the Stats as the Channel of the Broadcast reports them.
func (st Stats) Figures() Figures {
	f := Figures{
		Recipients: st.Recipients, Sent: st.Sent, Skipped: st.Skipped, Failed: st.Failed, Pending: st.Pending,
		Clicked: st.Clicked, Unsubscribed: st.Unsubscribed,
		ClickRate: st.ClickRate(), UnsubscribeRate: st.UnsubscribeRate(), FailureRate: st.FailureRate(),
		Revenue: st.Revenue,
	}
	if st.ReportsOpens() {
		f.Opened, f.OpenRate, f.ClickToOpenRate = &st.Opened, new(st.OpenRate()), new(st.ClickToOpenRate())
	}
	if st.ReportsDelivered() {
		f.Delivered = &st.Delivered
	}
	if st.ReportsUndelivered() {
		f.Undelivered = &st.Undelivered
	}
	if st.ReportsBounces() {
		f.Bounced, f.Complained, f.DeliveryRate = &st.Bounced, &st.Complained, new(st.DeliveryRate())
	}
	if st.ReportsCaptured() {
		f.Captured = &st.Captured
	}
	return f
}

// ReportsOpens says whether the Channel reports opens: email does, Telegram and SMS cannot tell.
// Without them there is no open count, open rate or click-to-open rate.
func (st Stats) ReportsOpens() bool { return st.onEmail() }

// ReportsDelivered says whether the Channel's provider reports a delivery: email's and SMS's do,
// Telegram's gives only the acceptance (Sent).
func (st Stats) ReportsDelivered() bool { return st.Channel != channels.Telegram }

// ReportsUndelivered says whether the Channel's provider reports a message it could not deliver
// (ADR 0079): SMS's does; email's says bounced instead.
func (st Stats) ReportsUndelivered() bool { return st.Channel == channels.SMS }

// ReportsBounces says whether the Channel reports bounces and complaints: email. The delivery rate
// goes with them: SMS reports delivered and undelivered, but no delivery rate.
func (st Stats) ReportsBounces() bool { return st.onEmail() }

// ReportsCaptured says whether the Channel has a Test inbox that captures its messages: email.
func (st Stats) ReportsCaptured() bool { return st.onEmail() }

// onEmail says whether the figures are an email Broadcast's: the Channel StatsFor set, or none
// (StatsOf, which reads them as email's).
func (st Stats) onEmail() bool {
	return st.Channel != channels.Telegram && st.Channel != channels.SMS
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

// PendingOfSending counts the recipients the Workspace's sending Broadcasts, held ones aside, still
// have to send: what they draw from the Send cap bucket before a Broadcast that has yet to start.
func PendingOfSending(ctx context.Context, s *ent.Scoped) (int, error) {
	return s.BroadcastRecipient().Query().
		Where(Pending(), broadcastrecipient.HasBroadcastWith(broadcast.StatusEQ(broadcast.StatusSending), broadcast.HoldReasonIsNil())).
		Count(ctx)
}

// StatsFor is StatsOf for the given Broadcasts, each figure set carrying its Broadcast's Channel.
func StatsFor(ctx context.Context, s *ent.Scoped, bs ...*ent.Broadcast) (map[int64]Stats, error) {
	ids := make([]int64, len(bs))
	for i, b := range bs {
		ids[i] = b.ID
	}
	out, err := StatsOf(ctx, s, ids...)
	if err != nil {
		return nil, err
	}
	for _, b := range bs {
		st := out[b.ID]
		st.Channel = b.Channel
		out[b.ID] = st
	}
	return out, nil
}

// StatsOf reads the figures of the given Broadcasts, whatever their number: a frozen Broadcast's
// from its BroadcastFigure and nothing live, the others' in two grouped queries over their rows,
// and every one's Revenue from its Orders. A Broadcast with no rows yet has zero figures.
func StatsOf(ctx context.Context, s *ent.Scoped, ids ...int64) (map[int64]Stats, error) {
	if len(ids) == 0 {
		return map[int64]Stats{}, nil
	}
	frozen, err := s.BroadcastFigure().Query().Where(broadcastfigure.BroadcastIDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
	}
	live := slices.DeleteFunc(slices.Clone(ids), func(id int64) bool {
		return slices.ContainsFunc(frozen, func(f *ent.BroadcastFigure) bool { return f.BroadcastID == id })
	})
	out, err := RowStatsOf(ctx, s, live...)
	if err != nil {
		return nil, err
	}
	for _, f := range frozen {
		out[f.BroadcastID] = Stats{
			Recipients: f.Recipients, Unsubscribed: f.Unsubscribed,
			Sent: f.Sent, Skipped: f.Skipped, Failed: f.Failed, Pending: f.Pending,
			Delivered: f.Delivered, Undelivered: f.Undelivered, Bounced: f.Bounced, Complained: f.Complained,
			Opened: f.Opened, Clicked: f.Clicked, Captured: f.Captured,
		}
	}
	revenue, err := orders.CreditedToBroadcasts(ctx, s, ids...)
	if err != nil {
		return nil, err
	}
	for id := range out {
		st := out[id]
		st.Revenue = revenue.Of(id)
		out[id] = st
	}
	return out, nil
}

// RowStatsOf counts the given Broadcasts' figures from their rows, Revenue aside, whether or not
// they are frozen: what message retention freezes (ADR 0143).
func RowStatsOf(ctx context.Context, s *ent.Scoped, ids ...int64) (map[int64]Stats, error) {
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
		Unsubscribed int   `json:"unsubscribed"`
	}
	if err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastIDIn(ids...)).
		GroupBy(broadcastrecipient.FieldBroadcastID).
		Aggregate(
			ent.As(ent.Count(), "total"),
			aggregates.CountSet(broadcastrecipient.FieldUnsubscribedAt, "unsubscribed"),
		).
		Scan(ctx, &recipients); err != nil {
		return nil, err
	}
	for _, r := range recipients {
		st := out[r.BroadcastID]
		st.Recipients, st.Unsubscribed = r.Total, r.Unsubscribed
		out[r.BroadcastID] = st
	}

	var messages []struct {
		BroadcastID int64                  `json:"broadcast_id"`
		Status      outboundmessage.Status `json:"status"`
		Count       int                    `json:"count"`
		Delivered   int                    `json:"delivered"`
		Undelivered int                    `json:"undelivered"`
		Bounced     int                    `json:"bounced"`
		Complained  int                    `json:"complained"`
		Opened      int                    `json:"opened"`
		Clicked     int                    `json:"clicked"`
		Captured    int                    `json:"captured"`
	}
	if err := s.OutboundMessage().Query().
		Where(outboundmessage.BroadcastIDIn(ids...)).
		GroupBy(outboundmessage.FieldBroadcastID, outboundmessage.FieldStatus).
		Aggregate(
			ent.As(ent.Count(), "count"),
			aggregates.CountSet(outboundmessage.FieldDeliveredAt, "delivered"),
			aggregates.CountSet(outboundmessage.FieldUndeliveredAt, "undelivered"),
			aggregates.CountSet(outboundmessage.FieldBouncedAt, "bounced"),
			aggregates.CountSet(outboundmessage.FieldComplainedAt, "complained"),
			aggregates.CountSet(outboundmessage.FieldOpenedAt, "opened"),
			aggregates.CountSet(outboundmessage.FieldClickedAt, "clicked"),
			aggregates.CountTrue(outboundmessage.FieldCaptured, "captured"),
		).
		Scan(ctx, &messages); err != nil {
		return nil, err
	}
	for _, m := range messages {
		st := out[m.BroadcastID]
		st.Delivered += m.Delivered
		st.Undelivered += m.Undelivered
		st.Bounced += m.Bounced
		st.Complained += m.Complained
		st.Opened += m.Opened
		st.Clicked += m.Clicked
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
	for id := range out {
		st := out[id]
		st.Pending = max(0, st.Recipients-st.Sent-st.Skipped-st.Failed)
		out[id] = st
	}
	return out, nil
}

// The unique constraint of a Broadcast's frozen figures (ADR 0106): no client can break it.
var _ = db.Declare(
	db.Fault("broadcast_figures_workspace_id_broadcast_id",
		"broadcastsunchecked.Freeze writes the one row ON CONFLICT DO NOTHING, so a prune run again keeps the figures it froze first"),
)
