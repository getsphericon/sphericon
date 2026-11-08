package outboundmessages

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
)

// Lasting report stamps retain the earliest human observation across asynchronous handling
// and retries (ADR 0113). An earlier observation may move a stamp earlier, but never clears it
// or counts a message twice. Machine observations are excluded before work is queued.

// StampOpened retains the earliest observed human open.
func StampOpened(ctx context.Context, s *ent.Scoped, messageID int64, at time.Time) (bool, error) {
	n, err := s.OutboundMessage().Update().
		Where(outboundmessage.ID(messageID), outboundmessage.Or(outboundmessage.OpenedAtIsNil(), outboundmessage.OpenedAtGT(at))).
		SetOpenedAt(at).
		Save(ctx)
	return n > 0, err
}

// StampClicked retains the earliest observed human click.
func StampClicked(ctx context.Context, s *ent.Scoped, messageID int64, at time.Time) (bool, error) {
	n, err := s.OutboundMessage().Update().
		Where(outboundmessage.ID(messageID), outboundmessage.Or(outboundmessage.ClickedAtIsNil(), outboundmessage.ClickedAtGT(at))).
		SetClickedAt(at).
		Save(ctx)
	return n > 0, err
}
