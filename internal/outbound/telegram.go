package outbound

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/bindings"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/telegramrender"
)

// ReasonContentTooLong fails one Telegram message whose rendered text is over the limit (ADR 0078).
const ReasonContentTooLong = "content_too_long"

// reasonContentEmpty fails a Telegram message whose text renders to nothing, which Telegram refuses.
const reasonContentEmpty = "content_empty"

// TelegramContent is Telegram's Message content (ADR 0078): a text in Telegram's HTML subset, with
// Liquid merge tags, and an optional unsubscribe button. Images and link buttons come with their
// own ticket.
type TelegramContent struct {
	Text string
	// UnsubscribeButton is the label of the callback button that is the opt-out, added as the
	// last row; "" adds none.
	UnsubscribeButton string
}

// Channel is Telegram.
func (TelegramContent) Channel() messaging.Channel { return messaging.ChannelTelegram }

// TelegramBots builds the bot a Telegram Integration is, for the Integration a Send route resolved.
// *messaging.Resolver satisfies it.
type TelegramBots interface {
	TelegramBot(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (messaging.TelegramBot, error)
}

// WithTelegramBots gives the module the bots Telegram messages are sent through. Without it the
// Telegram Channel has no Integration that can send.
func WithTelegramBots(b TelegramBots) Option { return func(m *Module) { m.bots = b } }

// telegramChannel is Telegram's part of the Outbound send: the renderer of the text and the adapter
// over the Integration's bot. The Contact's Bindings on the routed Integration are its Destinations.
type telegramChannel struct{ m *Module }

func (c telegramChannel) adapter(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (Adapter, error) {
	if c.m.bots == nil || integ == nil {
		return nil, messaging.ErrNoProvider
	}
	bot, err := c.m.bots.TelegramBot(ctx, s, integ)
	if err != nil {
		return nil, err
	}
	return telegramAdapter{bot: bot}, nil
}

// targets are the Bindings the Contact has on the routed Integration, its own and its Visitors',
// oldest first: the message goes to each chat, each passing Send-eligibility on its own.
func (telegramChannel) targets(ctx context.Context, s *ent.Scoped, req Request, route Route, _ string) ([]target, error) {
	contactID := req.contactID()
	if contactID == 0 || route.Integration == nil {
		return nil, nil
	}
	rows, err := s.Binding().Query().
		Where(
			binding.IntegrationID(route.Integration.ID),
			binding.Or(binding.ContactID(contactID), binding.HasVisitorWith(visitor.ContactID(contactID))),
		).
		Order(binding.ByBoundAt(), binding.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbound: list bindings: %w", err)
	}
	out := make([]target, len(rows))
	for i, b := range rows {
		out[i] = target{dest: b.Destination, chat: b.ChatID}
	}
	return out, nil
}

// telegramMessage is a rendered Telegram message: its text, as the Bot API takes it.
type telegramMessage struct{ text, unsubscribeButton string }

func telegramContentOf(content Content) (TelegramContent, error) {
	c, ok := content.(TelegramContent)
	if !ok {
		return TelegramContent{}, fmt.Errorf("outbound: %T is no Telegram content", content)
	}
	return c, nil
}

// render renders the text for one recipient. A text that does not render, renders to nothing or is
// over the limit once rendered is the message's own failure, the same on every attempt.
func (c telegramChannel) render(ctx context.Context, s *ent.Scoped, req Request, messageID int64, _ string, _ *ent.Workspace) (any, error) {
	content, err := telegramContentOf(req.Content)
	if err != nil {
		return nil, err
	}
	vars := Bindings(req.Contact, req.Variables)
	start, err := c.m.startLinkBinding(ctx, s, req, messageID, content.Text)
	if err != nil {
		return nil, err
	}
	maps.Copy(vars, start)
	text, err := telegramrender.Render(content.Text, vars)
	if err != nil {
		return nil, contentError{err}
	}
	switch n := telegramrender.VisibleLength(text); {
	case n == 0:
		return nil, contentError{errors.New(reasonContentEmpty)}
	case n > telegramrender.MaxText:
		return nil, contentError{errors.New(ReasonContentTooLong)}
	}
	return telegramMessage{text: text, unsubscribeButton: content.UnsubscribeButton}, nil
}

// telegramAdapter is Telegram's provider adapter over the bot of the routed Integration.
type telegramAdapter struct{ bot messaging.TelegramBot }

// DeliversOutside: a bot's messages reach Telegram.
func (telegramAdapter) DeliversOutside() bool { return true }

func (telegramAdapter) prepare(_ context.Context, _ *ent.Scoped, content Content) (sendAs, string, error) {
	if _, err := telegramContentOf(content); err != nil {
		return sendAs{}, "", err
	}
	return sendAs{}, "", nil
}

// send calls sendMessage for the target's chat. The receipt carries the id Telegram gave the
// message in that chat, which is how a press on its unsubscribe button finds the message again.
func (a telegramAdapter) send(ctx context.Context, _ sendAs, msg any, to target, _ messaging.Origin) (messaging.Receipt, error) {
	m, ok := msg.(telegramMessage)
	if !ok {
		return messaging.Receipt{}, fmt.Errorf("outbound: %T is no Telegram message", msg)
	}
	sent, err := a.bot.SendMessage(ctx, messaging.TelegramMessage{ChatID: to.chat, Text: m.text, UnsubscribeButton: m.unsubscribeButton})
	if err != nil {
		return messaging.Receipt{}, classifyTelegram(err)
	}
	return messaging.Receipt{MessageID: strconv.FormatInt(sent.MessageID, 10)}, nil
}

// classifyTelegram sorts a refusal of the Bot API by whom it concerns (ADR 0079), reading the code
// and the description together. A 401 is the bot's (a Hold), a 429 the rate (a Deferral of at least
// retry_after), a deactivated user or a missing chat the chat's (the Binding goes), any other 400
// the message's own (it fails with Telegram's description). Anything else, a network error or a
// 5xx included, is left as it came: a retryable error.
func classifyTelegram(err error) error {
	te, ok := errors.AsType[*messaging.TelegramError](err)
	if !ok {
		return err
	}
	desc := strings.ToLower(te.Description)
	switch {
	case te.Code == http.StatusUnauthorized:
		return holdError{reason: HoldProviderRefused}
	case te.Code == http.StatusTooManyRequests:
		return deferError{wait: max(te.RetryAfter, DeferralBackoff), err: err}
	case te.Code == http.StatusForbidden && strings.Contains(desc, "user is deactivated"),
		te.Code == http.StatusBadRequest && strings.Contains(desc, "chat not found"):
		return unreachableError{err}
	case te.Code == http.StatusBadRequest:
		return contentError{errors.New(te.Description)}
	default:
		return err
	}
}

// unreachable removes the Binding of a chat Telegram no longer reaches, and records telegram.unbound.
func (c telegramChannel) unreachable(ctx context.Context, s *ent.Scoped, pub events.Publisher, integ *ent.Integration, to target, messageID int64) error {
	return bindings.Remove(ctx, s, pub, integ, to.dest, fmt.Sprintf("%s:gone:%d:%s", events.NameTelegramUnbound, messageID, to.dest))
}

// publishSent publishes telegram.sent, with the Broadcast or Automation the message came from.
func (telegramAdapter) publishSent(ctx context.Context, pub events.Publisher, f sentFact) error {
	e := &events.TelegramSent{
		WorkspaceID:       f.workspaceID,
		ContactID:         f.req.contactID(),
		Destination:       f.dest,
		BroadcastID:       f.req.Ref.BroadcastID,
		AutomationID:      f.req.Ref.AutomationID,
		OutboundMessageID: f.messageID,
		Surface:           string(f.req.Kind),
		SendingSource:     f.req.Source,
		DedupID:           fmt.Sprintf("%s:%d", events.NameTelegramSent, f.messageID),
	}
	if f.req.Contact != nil && f.req.Contact.Email != nil {
		e.Email = *f.req.Contact.Email
	}
	if f.integrationID != 0 {
		e.IntegrationID = f.integrationID
	}
	return pub.Publish(ctx, e)
}
