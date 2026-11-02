package ingest

import (
	"context"
	"slices"
	"strings"

	"github.com/getsphericon/sphericon/internal/bindings"
	"github.com/getsphericon/sphericon/internal/preset"
)

// telegram reads the messages people send a bot (ADR 0076). A `/start <token>` binds the chat to
// the Contact the token was issued for. Any other message from a Destination Bound to a Contact is
// recorded as telegram.received, unless it is a command with a meaning of its own. The bot never
// answers.
type telegram struct{ Ignore }

// InboundMessage redeems the message as a start link when it is one, and records it as received
// when it means nothing else.
func (telegram) InboundMessage(ctx context.Context, tx Tx, m preset.InboundMessage) error {
	if err := bindings.Redeem(ctx, tx.Scoped, tx.Pub, tx.Integration, tx.Now, bindings.Redemption{
		Destination: m.Destination, ChatID: m.ChatID, Text: m.Text, DedupKey: tx.DedupKey(m.ID),
	}); err != nil {
		return err
	}
	if hasOwnMeaning(m.Text) {
		return nil
	}
	return bindings.Receive(ctx, tx.Scoped, tx.Pub, tx.Integration, bindings.Message{
		Destination: m.Destination, Text: m.Text, DedupKey: tx.DedupKey(m.ID),
	})
}

// ownMeaning are the commands the core reads itself, so they are not a Contact's words. The
// opt-out keywords join them with the opt-out ticket.
var ownMeaning = []string{"/start", "/stop"}

// hasOwnMeaning reports whether text is one of the commands in ownMeaning, with or without an
// argument and addressed to a bot (`/start@name`).
func hasOwnMeaning(text string) bool {
	command, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	name, _, _ := strings.Cut(strings.ToLower(command), "@")
	return slices.Contains(ownMeaning, name)
}
