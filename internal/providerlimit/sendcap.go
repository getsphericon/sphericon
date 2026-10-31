package providerlimit

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

// SendCaps is whether the Instance has Send caps at all (ADR 0067; the saas Mode has them). The
// container decides it once, from the Mode, and hands this one value to every module that enforces,
// shows or sets a Send cap. On an Instance without them a stored Send cap or a stored default has
// no effect, and setting either is not found, whatever the License key.
type SendCaps bool

// ErrNoSendCaps refuses a change of a Send cap or of the default Send cap on an Instance that has
// no Send caps: there is nothing to change there, so it is not found.
var ErrNoSendCaps = validation.NotFound("providerlimit: this instance has no send caps", i18n.ErrorsSendCapsAbsent)

// Of is the Send cap that applies to the Workspace ws: its own on an Instance that has Send caps,
// none elsewhere, whatever is stored.
func (c SendCaps) Of(ws *ent.Workspace) *int {
	if !c {
		return nil
	}
	return ws.SendCap
}

// Check refuses a change of a Send cap on an Instance that has none (ErrNoSendCaps).
func (c SendCaps) Check() error {
	if !c {
		return ErrNoSendCaps
	}
	return nil
}

// SendCapStatus is a Workspace's Send cap as shown to its members and agents: the messages it may
// send per rolling 24 hours and the messages it sent over the last 24 hours. Each surface maps it
// to its own DTO.
type SendCapStatus struct {
	PerDay  int
	Sent24h int
}

// SentLast24h counts the messages the Workspace of s sent over the 24 hours before now, through
// any route, Transactional included: every message the Send cap was spent for and the provider
// accepted (ADR 0067).
func SentLast24h(ctx context.Context, s *ent.Scoped, now time.Time) (int, error) {
	return s.OutboundMessage().Query().
		Where(
			outboundmessage.StatusEQ(outboundmessage.StatusSent),
			outboundmessage.SentAtGT(now.Add(-window)),
		).
		Count(ctx)
}
