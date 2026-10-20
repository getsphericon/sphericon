package broadcasts

import (
	"context"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
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
}

// Processed counts the recipients whose send has ended.
func (st Stats) Processed() int { return st.Recipients - st.Pending }

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
			nonNull(broadcastrecipient.FieldOpenedAt, "opened"),
			nonNull(broadcastrecipient.FieldClickedAt, "clicked"),
			nonNull(broadcastrecipient.FieldUnsubscribedAt, "unsubscribed"),
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
	}
	if err := s.OutboundMessage().Query().
		Where(outboundmessage.BroadcastIDIn(ids...)).
		GroupBy(outboundmessage.FieldBroadcastID, outboundmessage.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &messages); err != nil {
		return nil, err
	}
	for _, m := range messages {
		st := out[m.BroadcastID]
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
	for id, st := range out {
		st.Pending = max(0, st.Recipients-st.Sent-st.Skipped-st.Failed)
		out[id] = st
	}
	return out, nil
}

// nonNull counts the rows whose column is set, under alias.
func nonNull(column, alias string) ent.AggregateFunc {
	return func(sel *sql.Selector) string {
		return sql.As(sql.Count(sel.C(column)), alias)
	}
}
