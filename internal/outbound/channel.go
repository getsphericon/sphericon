package outbound

import (
	"context"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// Content is a Message content value (GLOSSARY, ADR 0078): what one message on one Channel says,
// its shape the Channel's own. Its Channel is the Channel the send goes out on, so a Request
// cannot name one Channel and carry another's content.
type Content interface {
	Channel() messaging.Channel
}

// A channel is one Channel's part of the Outbound send (ADR 0079): its renderer and its provider
// adapter, the only parts of the send that differ per Channel. Every step of the pipeline is
// Send's; adding a Channel adds its entry to Module.channels and touches none of them.
type channel interface {
	// adapter builds the Channel's provider adapter for the Integration a Send route resolved.
	// integ is nil when the Channel has no Integration to send through: a test double's sender
	// may still answer. messaging.ErrNoProvider when none can be built.
	adapter(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (Adapter, error)
	// render renders req's content for one recipient, whose Outbound message is messageID and
	// Destination dest, into the message the Channel's adapter sends. A contentError is the
	// message's own and ends the send as Failed; any other error is retried.
	render(ctx context.Context, s *ent.Scoped, req Request, messageID int64, dest string, ws *ent.Workspace) (any, error)
}

// Adapter is a Channel's provider adapter, built for the Integration of one Send route. Only the
// Channels of this package implement it.
type Adapter interface {
	// DeliversOutside is the sender's own word that its messages leave sphericon (ADR 0058): a send
	// through one that keeps them is recorded as captured.
	DeliversOutside() bool
	// prepare runs the Channel's own checks of a send of content through the adapter, before
	// anything is claimed: hold says why it cannot go now, else as is who it goes out as.
	prepare(ctx context.Context, s *ent.Scoped, content Content) (as sendAs, hold string, err error)
	// send hands one message its Channel's renderer made to the provider. An error wrapping a
	// holdError holds the send.
	send(ctx context.Context, as sendAs, msg any, dest string, origin messaging.Origin) (messaging.Receipt, error)
	// publishSent publishes the Channel's Events of a message recorded sent, in the transaction that
	// records it: its sent Event and, when the provider confirmed delivery on acceptance, its
	// delivered Event.
	publishSent(ctx context.Context, pub events.Publisher, f sentFact) error
}

// sendAs is who a message goes out as, as its adapter's prepare found it: on email the From
// address, its display name and the Sending domain stamped on the Outbound message. A Channel
// with no sender identity of its own leaves it empty.
type sendAs struct {
	address       string
	name          string
	sendingDomain string
}

// sentFact is what a Channel's Events of a sent message say.
type sentFact struct {
	workspaceID int64
	req         Request
	messageID   int64
	dest        string
	as          sendAs
	receipt     messaging.Receipt
	captured    bool
}

// holdError is a provider adapter's word that the send cannot go now for a reason that clears by
// itself (a Hold): nothing left, the claim is dropped and the same Request runs again later.
type holdError struct{ reason string }

func (e holdError) Error() string { return "outbound: held: " + e.reason }

// heldBy is the Hold reason err carries, "" when it carries none.
func heldBy(err error) string {
	if h, ok := errors.AsType[holdError](err); ok {
		return h.reason
	}
	return ""
}

// contentError marks a render failure that comes from the message itself (a template that does
// not render, a link that cannot be rewritten): it is the same on every attempt, so the send ends
// as Failed. Any other render error is a failure of what the render reads (the tracking host
// lookup), which a retry may get past.
type contentError struct{ err error }

func (e contentError) Error() string { return e.err.Error() }
func (e contentError) Unwrap() error { return e.err }

func isContentError(err error) bool {
	var ce contentError
	return errors.As(err, &ce)
}
