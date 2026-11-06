package outboundmessages

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
)

// The engagement of a recipient with an Outbound message (ADR 0053): the first human open and the
// first human click are stamped on the message, whichever surface sent it, the way its delivery
// results are. Each is set once, by a conditional update, and reports whether this call set it, so
// a repeated hit (a mailbox that fetches the pixel twice, a double click) is counted exactly once
// and a stamp is never replaced or cleared. The caller publishes the engagement Event in the same
// transaction, and records a machine hit (which stamps nothing) as an Event only.

// StampOpened stamps the message as opened, only while it is not.
func StampOpened(ctx context.Context, s *ent.Scoped, messageID int64) (bool, error) {
	n, err := s.OutboundMessage().Update().
		Where(outboundmessage.ID(messageID), outboundmessage.OpenedAtIsNil()).
		SetOpenedAt(time.Now()).
		Save(ctx)
	return n > 0, err
}

// StampClicked stamps the message as clicked, only while it is not.
func StampClicked(ctx context.Context, s *ent.Scoped, messageID int64) (bool, error) {
	n, err := s.OutboundMessage().Update().
		Where(outboundmessage.ID(messageID), outboundmessage.ClickedAtIsNil()).
		SetClickedAt(time.Now()).
		Save(ctx)
	return n > 0, err
}
