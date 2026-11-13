package broadcastsunchecked

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastfigure"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// The steps of message retention's prune of a finished Broadcast (ADR 0143), which the use case
// internal/usecase/messageretention runs in their order: Freeze, then RecipientsOf and
// DeleteRecipients in the transaction that deletes those recipients' messages, then Pruned once
// its last message is gone. Freezing a Broadcast that still sends would stop its figures, and
// deleting recipients before their messages would make them read as pending, so none runs alone.

// Freeze writes the Broadcast's figures, as its rows count them, to its BroadcastFigure, unless
// one is there already: a figures row is written once and never recomputed, so a prune cut short
// and run again keeps the figures it froze before it deleted anything.
func Freeze(ctx context.Context, s *ent.Scoped, id int64) error {
	frozen, err := s.BroadcastFigure().Query().Where(broadcastfigure.BroadcastID(id)).Exist(ctx)
	if err != nil || frozen {
		return err
	}
	stats, err := broadcasts.RowStatsOf(ctx, s, id)
	if err != nil {
		return err
	}
	st := stats[id]
	return s.BroadcastFigure().Create().
		SetBroadcastID(id).
		SetRecipients(st.Recipients).SetUnsubscribed(st.Unsubscribed).
		SetSent(st.Sent).SetSkipped(st.Skipped).SetFailed(st.Failed).SetPending(st.Pending).
		SetDelivered(st.Delivered).SetUndelivered(st.Undelivered).
		SetBounced(st.Bounced).SetComplained(st.Complained).
		SetOpened(st.Opened).SetClicked(st.Clicked).SetCaptured(st.Captured).
		OnConflictColumns(broadcastfigure.FieldWorkspaceID, broadcastfigure.FieldBroadcastID).
		DoNothing().
		Exec(ctx)
}

// RecipientsOf lists up to limit of the Broadcast's recipients.
func RecipientsOf(ctx context.Context, s *ent.Scoped, id int64, limit int) ([]int64, error) {
	return s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastID(id)).
		Limit(limit).
		IDs(ctx)
}

// DeleteRecipients deletes the recipients, once their messages are gone.
func DeleteRecipients(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.BroadcastRecipient().Delete().Where(broadcastrecipient.IDIn(ids...)).Exec(ctx)
	return err
}

// Pruned marks the frozen Broadcast pruned at now: none of its recipients and messages is left. One
// marked already keeps its moment.
func Pruned(ctx context.Context, s *ent.Scoped, id int64, now time.Time) error {
	err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := s.BroadcastFigure().Update().
			Where(broadcastfigure.BroadcastID(id), broadcastfigure.PrunedAtIsNil()).
			SetPrunedAt(now).
			Save(ctx)
		return n == 1, err
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil
	}
	return err
}
