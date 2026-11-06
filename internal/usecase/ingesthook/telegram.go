package ingesthook

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/bindings"
	"github.com/getsphericon/sphericon/internal/domain/contacts/optout"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/logging"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/preset"
)

// telegram reads what happens at a bot (ADR 0076, ADR 0077): the messages people send it, the
// buttons they press and their blocking or unblocking it. A `/start <token>` binds the chat to the
// Contact the token was issued for, and any `/start` lifts the sender's opt-out. `/stop`, a STOP
// keyword, the unsubscribe button and a block are the sender's Channel-wide opt-out. Any other
// message from a Destination Bound to a Contact is recorded as telegram.received. The bot never
// answers a message; a button press is only acknowledged.
type telegram struct {
	Ignore

	bots Bots
}

// InboundMessage reads a pressed button or a written message for what it means.
func (t telegram) InboundMessage(ctx context.Context, tx Tx, m preset.InboundMessage) error {
	if m.CallbackID != "" {
		return t.pressed(ctx, tx, m)
	}
	if err := bindings.Redeem(ctx, tx.Scoped, tx.Pub, tx.Integration, tx.Now, bindings.Redemption{
		Destination: m.Destination, ChatID: m.ChatID, Text: m.Text, DedupKey: tx.DedupKey(m.ID),
	}); err != nil {
		return err
	}
	switch command(m.Text) {
	case "/start":
		return unsubscribes.LiftTelegram(ctx, tx.Scoped, m.Destination)
	case "/stop":
		return t.stop(ctx, tx, m, events.UnsubscribedByKeyword, nil)
	}
	if optout.IsStopKeyword(m.Text) {
		return t.stop(ctx, tx, m, events.UnsubscribedByKeyword, nil)
	}
	return bindings.Receive(ctx, tx.Scoped, tx.Pub, tx.Integration, bindings.Message{
		Destination: m.Destination, Text: m.Text, DedupKey: tx.DedupKey(m.ID),
	})
}

// pressed reads a button press. The unsubscribe button is the opt-out, attributed to the Broadcast
// or Automation of the message it was pressed on when that resolves to an Outbound message; any
// press is answered once the facts are written, so the person's client stops waiting.
func (t telegram) pressed(ctx context.Context, tx Tx, m preset.InboundMessage) error {
	if m.Payload == messaging.TelegramUnsubscribePayload {
		msg, err := outbound.FindTelegramMessage(ctx, tx.Scoped, tx.Integration.ID, m.Destination, m.MessageID)
		if err != nil {
			return err
		}
		if err := t.stop(ctx, tx, m, events.UnsubscribedByButton, msg); err != nil {
			return err
		}
	}
	tx.After(func(ctx context.Context) { t.answer(ctx, tx, m.CallbackID) })
	return nil
}

// answer acknowledges a button press, best effort: the press is recorded and Telegram stops
// waiting for an answer by itself after a while.
func (t telegram) answer(ctx context.Context, tx Tx, callbackID string) {
	log := logging.FromContext(ctx)
	if t.bots == nil {
		return
	}
	bot, err := t.bots.Bot(tx.Integration)
	if err != nil {
		log.Warn("ingest: cannot open the bot to answer a button press", "integration_id", tx.Integration.ID, "err", err)
		return
	}
	if err := bot.AnswerCallbackQuery(ctx, callbackID); err != nil {
		log.Warn("ingest: answering a button press failed", "integration_id", tx.Integration.ID, "err", err)
	}
}

func (telegram) stop(ctx context.Context, tx Tx, m preset.InboundMessage, via string, msg *ent.OutboundMessage) error {
	return optout.StopTelegram(ctx, tx.Scoped, tx.Pub, optout.TelegramStop{
		Destination: m.Destination, IntegrationID: tx.Integration.ID, Via: via, Message: msg,
		Now: tx.Now, DedupKey: tx.DedupKey(m.ID),
	})
}

// ReachabilityChange reads Telegram saying the person blocked the bot (their Channel-wide
// opt-out), unblocked it (which lifts it) or deleted their account (the Binding goes, no opt-out).
func (telegram) ReachabilityChange(ctx context.Context, tx Tx, c preset.ReachabilityChange) error {
	switch {
	case c.Reachable:
		return unsubscribes.LiftTelegram(ctx, tx.Scoped, c.Destination)
	case c.Reason == preset.Deactivated:
		return bindings.Remove(ctx, tx.Scoped, tx.Pub, tx.Integration, c.Destination, tx.DedupKey(c.ID))
	default:
		return optout.StopTelegram(ctx, tx.Scoped, tx.Pub, optout.TelegramStop{
			Destination: c.Destination, IntegrationID: tx.Integration.ID, Via: events.UnsubscribedByBlock,
			Now: tx.Now, DedupKey: tx.DedupKey(c.ID),
		})
	}
}

// command is the lower-cased name of the command text starts with, without the argument or a bot's
// name (`/Start@name token` is `/start`), "" when text is no command.
func command(text string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	name, _, _ := strings.Cut(strings.ToLower(first), "@")
	if !strings.HasPrefix(name, "/") {
		return ""
	}
	return name
}
