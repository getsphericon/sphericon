package ingest

import (
	"context"

	"github.com/getsphericon/sphericon/internal/bindings"
	"github.com/getsphericon/sphericon/internal/preset"
)

// telegram reads the messages people send a bot: a `/start <token>` binds the chat to the Contact
// the token was issued for (ADR 0076). Any other text records nothing yet, and the bot never
// answers.
type telegram struct{ Ignore }

// InboundMessage redeems the message as a start link when it is one.
func (telegram) InboundMessage(ctx context.Context, tx Tx, m preset.InboundMessage) error {
	return bindings.Redeem(ctx, tx.Scoped, tx.Pub, tx.Integration, tx.Now, bindings.Redemption{
		Destination: m.Destination, ChatID: m.ChatID, Text: m.Text, DedupKey: tx.DedupKey(m.ID),
	})
}
