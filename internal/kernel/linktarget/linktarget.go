// Package linktarget names what an unsubscribe link and a confirmation link carry: the opt-out or
// the Confirmation they record, signed into the link's token by internal/tracking and recorded by
// internal/usecase/unsubscriberecording and internal/domain/contacts/confirmations. It sits below
// both, so recording one does not import the tokens.
package linktarget

import "github.com/getsphericon/sphericon/internal/kernel/channels"

// Unsubscribe is the self-describing payload an unsubscribe link carries. The
// opt-out is destination-keyed (ADR 0001), so the link records against the
// destination directly — no contact-row lookup, and it survives the contact being
// deleted between send and click. BroadcastID is set only for broadcast sends
// (for per-broadcast attribution); Source is the sending-source string the opt-out
// is scoped to ("broadcasts" / "automation:<id>" / "everything"). Channel is the Channel the
// opt-out is recorded on: a link in a Telegram message unsubscribes on Telegram only (ADR 0077),
// and Destination is that Channel's (the email address, the Telegram user id). OutboundMessageID is
// the message the link sits in, 0 for a link that came from none (a message sent before the link
// named it, or an opt-out made without one): the email.unsubscribed Event is about that message.
type Unsubscribe struct {
	Channel           channels.Channel
	Source            string
	Destination       string
	WorkspaceID       int64
	ContactID         int64
	BroadcastID       int64
	OutboundMessageID int64
}

// Confirmation is the payload a double-opt-in confirmation link carries (ADR
// 0013). Like the opt-out, the confirmation is destination-keyed and not
// source-scoped: confirming an address makes it mailable across sources, so the
// link records against the destination directly (no contact-row lookup) and
// survives the contact being deleted between send and click.
type Confirmation struct {
	Destination string
	WorkspaceID int64
	ContactID   int64
}
