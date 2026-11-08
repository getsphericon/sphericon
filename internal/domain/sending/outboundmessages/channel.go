package outboundmessages

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Content is a Message content value (GLOSSARY, ADR 0078): what one message on one Channel says,
// its shape the Channel's own. Its Channel is the Channel the send goes out on, so a Request
// cannot name one Channel and carry another's content.
type Content interface {
	Channel() channels.Channel
}

// A channel is one Channel's part of the Outbound send (ADR 0079): its renderer and its provider
// adapter, the only parts of the send that differ per Channel. Every step of the pipeline is
// Send's; adding a Channel adds its entry to Module.channels and touches none of them.
type channel interface {
	// adapter builds the Channel's provider adapter for the Integration a Send route resolved.
	// integ is nil when the Channel has no Integration to send through: a test double's sender
	// may still answer. messaging.ErrNoProvider when none can be built.
	adapter(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (Adapter, error)
	// targets resolves where the message goes, after the claim: one target per Destination the
	// Channel reaches the Request's recipient at. dest is the Request's own normalized Destination
	// (on email the address, empty for a Contact with none); a Channel of Bindings ignores it and
	// reads the recipient's Bindings on the route's Integration. None is a Skipped no_destination.
	targets(ctx context.Context, s *ent.Scoped, req Request, route Route, dest string) ([]target, error)
	// render renders req's content for one recipient, whose Outbound message is messageID and
	// Destination dest, into the message the Channel's adapter sends. A contentError is the
	// message's own and ends the send as Failed; any other error is retried.
	render(ctx context.Context, s *ent.Scoped, req Request, messageID int64, dest string, ws *ent.Workspace) (any, error)
	// unreachable drops a target the provider said no longer reaches anyone (an unreachableError
	// of send): a Channel of Bindings removes the Binding and records that it is gone, SMS
	// suppresses the number. skip is the Skipped reason Send-eligibility now gives the target
	// (no_destination, suppressed). It runs in a transaction of its own, outside the send's claim.
	unreachable(ctx context.Context, s *ent.Scoped, pub events.Publisher, integ *ent.Integration, req Request, to target, messageID int64) (skip string, err error)
	// optedOut records the Channel-wide opt-out of a target whose provider refused the message
	// because the person stopped at the provider (an optedOutError of send: a blocked bot, a number
	// that texted STOP to the sender), attributed to msg, the message that found out (ADR 0077). It
	// runs in a transaction of its own, outside the send's claim.
	optedOut(ctx context.Context, s *ent.Scoped, pub events.Publisher, integ *ent.Integration, msg *ent.OutboundMessage, to target, now time.Time) error
}

// target is one place a message goes: dest is its Destination, the key of Send-eligibility and what
// the message records, and chat the provider's own address for it when that differs (a Telegram
// chat id), "" when the Destination is the address.
type target struct {
	dest string
	chat string
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
	// send hands one message its Channel's renderer made to the provider, for one target. An error
	// wrapping a holdError holds the send.
	send(ctx context.Context, as sendAs, msg any, to target, origin messaging.Origin) (messaging.Receipt, error)
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
	observedAt  time.Time
	workspaceID int64
	req         Request
	messageID   int64
	dest        string
	as          sendAs
	receipt     messaging.Receipt
	captured    bool
	// integrationID is the Integration the message went through, 0 without one.
	integrationID int64
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

// deferError is a provider adapter's word that the provider asked for slower calls and said for
// how long (Telegram's 429 with retry_after): nothing left, the send is deferred by at least wait.
type deferError struct {
	wait time.Duration
	err  error
}

func (e deferError) Error() string { return "outbound: deferred: " + e.err.Error() }
func (e deferError) Unwrap() error { return e.err }

// unreachableError is a provider adapter's word that the target reaches no one any more (Telegram's
// "user is deactivated", "chat not found", a number that is no mobile one): the Channel drops it (the
// Binding goes, the number is suppressed), and the message goes to the others.
type unreachableError struct{ err error }

func (e unreachableError) Error() string { return "outbound: unreachable: " + e.err.Error() }
func (e unreachableError) Unwrap() error { return e.err }

// optedOutError is a provider adapter's word that the person at the target stopped the Workspace's
// messages at the provider (Telegram's "bot was blocked by the user", Twilio's 21610 to a number that
// texted STOP): the Channel's opt-out is recorded (channel.optedOut), and the message goes on to the
// others.
type optedOutError struct{ err error }

func (e optedOutError) Error() string { return "outbound: opted out: " + e.err.Error() }
func (e optedOutError) Unwrap() error { return e.err }

// chatRefusedError is a provider adapter's word that one target refuses the message for good and
// that the refusal is that target's alone (Telegram's 403 "bot can't initiate conversation", an
// invalid number on SMS): the message goes on to the other targets. It fails, with the description,
// only when none took it. A description that starts with ReasonMessageRefused fails the message
// alone: its source goes on (Result.MessageOnly).
type chatRefusedError struct{ description string }

func (e chatRefusedError) Error() string { return e.description }

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
