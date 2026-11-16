package callbackinterpretation

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/channelstop"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
)

// telegram reads what happens at a bot (ADR 0076, ADR 0077): the messages people send it, the
// buttons they press and their blocking or unblocking it. A `/start <token>` binds the chat to the
// Contact the token was issued for, and any `/start` lifts the sender's opt-out. `/stop`, a STOP
// keyword, the unsubscribe button and a block are the sender's Channel-wide opt-out. Any other
// message from a Destination Bound to a Contact is recorded as telegram.received. The bot never
// answers a message; a button press is only acknowledged.
type telegram struct {
	ignore

	integ *integrations.Module
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
		return t.resume(ctx, tx, m.Destination, m.ID, m.OccurredAt)
	case "/stop":
		return t.stop(ctx, tx, m, events.UnsubscribedByKeyword, nil)
	}
	if channelstop.IsStopKeyword(m.Text) {
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
		msg, err := outboundmessages.FindTelegramMessage(ctx, tx.Scoped, tx.Integration.ID, m.Destination, m.MessageID)
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
	bot, err := t.integ.Bot(tx.Integration)
	if err != nil {
		slog.WarnContext(ctx, "ingest: cannot open the bot to answer a button press", "integration_id", tx.Integration.ID, "err", err)
		return
	}
	if err := bot.AnswerCallbackQuery(ctx, callbackID); err != nil {
		slog.WarnContext(ctx, "ingest: answering a button press failed", "integration_id", tx.Integration.ID, "err", err)
	}
}

func (telegram) stop(ctx context.Context, tx Tx, m preset.InboundMessage, via string, msg *ent.OutboundMessage) error {
	e := &events.TelegramStopObserved{
		WorkspaceID: tx.Scoped.WorkspaceID(), Destination: m.Destination, IntegrationID: tx.Integration.ID,
		Sequence: sequence(m.ID), Via: via, ObservedAt: occurred(tx, m.OccurredAt), DedupID: tx.DedupKey(m.ID),
	}
	if msg != nil {
		e.OutboundMessageID, e.ContactID = msg.ID, lo.FromPtr(msg.ContactID)
		e.BroadcastID, e.AutomationID = lo.FromPtr(msg.BroadcastID), lo.FromPtr(msg.AutomationID)
	}
	return tx.Pub.Publish(ctx, e)
}

// ReachabilityChange reads Telegram saying the person blocked the bot (their Channel-wide
// opt-out), unblocked it (which lifts it) or deleted their account (the Binding goes, no opt-out).
func (t telegram) ReachabilityChange(ctx context.Context, tx Tx, c preset.ReachabilityChange) error {
	switch {
	case c.Reachable:
		return t.resume(ctx, tx, c.Destination, c.ID, c.OccurredAt)
	case c.Reason == preset.Deactivated:
		return bindings.Remove(ctx, tx.Scoped, tx.Pub, tx.Integration, c.Destination, tx.DedupKey(c.ID))
	default:
		return tx.Pub.Publish(ctx, &events.TelegramStopObserved{
			WorkspaceID: tx.Scoped.WorkspaceID(),
			Destination: c.Destination, IntegrationID: tx.Integration.ID, Sequence: sequence(c.ID), Via: events.UnsubscribedByBlock,
			ObservedAt: occurred(tx, c.OccurredAt), DedupID: tx.DedupKey(c.ID),
		})
	}
}

// sequence is the bot's update_id an update's id is (the Preset's id): it orders two observations
// of one bot (ADR 0154). The engine has checked the id is the update's.
func sequence(id string) int64 {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// occurred is when an observation occurred: the provider's time where it gives one, else when it
// was received.
func occurred(tx Tx, provider time.Time) time.Time {
	if provider.IsZero() {
		return tx.Now
	}
	return provider
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

func (telegram) resume(ctx context.Context, tx Tx, destination, id string, at time.Time) error {
	return tx.Pub.Publish(ctx, &events.TelegramResumeObserved{
		WorkspaceID: tx.Scoped.WorkspaceID(), Destination: destination, IntegrationID: tx.Integration.ID,
		Sequence: sequence(id), DedupID: tx.DedupKey(id), ObservedAt: occurred(tx, at),
	})
}
