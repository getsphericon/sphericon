package outboundmessages

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/smsrender"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// SMSContent is SMS's Message content (ADR 0078): one plain text with Liquid merge tags, no media,
// nothing appended. Its links go out as short tracked links (ADR 0083) unless NoLinkTracking.
type SMSContent struct {
	Text string
	// NoLinkTracking keeps the links as written. Negated so that the zero value is the default.
	NoLinkTracking bool
}

// Channel is SMS.
func (SMSContent) Channel() channels.Channel { return channels.SMS }

// SMSSenders builds the sender an SMS Integration is, for the Integration a Send route resolved.
// *messaging.Resolver satisfies it.
type SMSSenders interface {
	SMSSender(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (messaging.SMSSender, error)
}

// HookURLs names the Ingest hook of an Integration: where its provider posts the delivery reports
// of the messages sent through it.
type HookURLs interface {
	HookURL(row *ent.Integration) string
}

// WithSMSSenders gives the module the senders SMS messages go through and the hooks their delivery
// reports are posted to. Without it the SMS Channel has no Integration that can send.
func WithSMSSenders(senders SMSSenders, hooks HookURLs) Option {
	return func(m *Module) { m.sms, m.hooks = senders, hooks }
}

// smsChannel is SMS's part of the Outbound send: the renderer of the text and the adapter over the
// Integration's account. The Contact's phone, in E.164, is its Destination.
type smsChannel struct{ m *Module }

func (c smsChannel) adapter(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (Adapter, error) {
	if c.m.sms == nil || c.m.hooks == nil || integ == nil {
		return nil, messaging.ErrNoProvider
	}
	sender, err := c.m.sms.SMSSender(ctx, s, integ)
	if err != nil {
		return nil, err
	}
	return smsAdapter{sender: sender, statusCallback: c.m.hooks.HookURL(integ)}, nil
}

// targets is the Contact's phone: one number, which the Request carries as its Destination.
func (smsChannel) targets(_ context.Context, _ *ent.Scoped, _ Request, _ Route, dest string) ([]target, error) {
	if dest == "" {
		return nil, nil
	}
	return []target{{dest: dest}}, nil
}

// unreachable suppresses a number the provider reports reaches no handset, like a hard bounce: no
// send on the Channel goes to it again, and the message is Skipped as suppressed.
func (smsChannel) unreachable(ctx context.Context, s *ent.Scoped, pub events.Publisher, _ *ent.Integration, req Request, to target, messageID int64) (string, error) {
	err := pub.Publish(ctx, &events.SmsDestinationUnreachable{WorkspaceID: s.WorkspaceID(), Phone: to.dest, ContactID: req.contactID(), DedupID: fmt.Sprintf("sms-send:%d/unreachable", messageID)})
	return eligibility.ReasonSuppressed, err
}

// optedOut reports the provider's semantic verdict without attributing a prior stop to this send.
func (smsChannel) optedOut(ctx context.Context, s *ent.Scoped, pub events.Publisher, _ *ent.Integration, msg *ent.OutboundMessage, to target, at time.Time) error {
	return pub.Publish(ctx, &events.SmsStopObserved{WorkspaceID: s.WorkspaceID(), Phone: to.dest, Via: events.UnsubscribedByRefusedSend, DedupID: fmt.Sprintf("sms-send:%d/opted-out", msg.ID), ObservedAt: at})
}

// smsMessage is a rendered text, as the provider takes it.
type smsMessage struct{ body string }

// render renders the text for one recipient: Liquid with the Contact's and the Enrollment's merge
// tags, and `unsubscribe_url` when the text uses it. Then every link but the unsubscribe link
// becomes a short tracked link of the message (ADR 0083), unless the content keeps its links; a Test
// send (messageID 0) gets the Workspace's message-less short links, so it shows the same form. A
// text that does not render, or renders to nothing, is the message's own failure, the same on every
// attempt.
func (c smsChannel) render(ctx context.Context, s *ent.Scoped, req Request, messageID int64, dest string, ws *ent.Workspace) (any, error) {
	content, ok := req.Content.(SMSContent)
	if !ok {
		return nil, fmt.Errorf("outbound: %T is no SMS content", req.Content)
	}
	vars := Bindings(req.Contact, req.Variables)
	if err := c.m.bindUnsubscribeURL(ctx, s, req, messageID, dest, ws, channels.SMS, content.Text, vars); err != nil {
		return nil, err
	}
	body, err := smsrender.Render(content.Text, vars)
	if err != nil {
		return nil, contentError{err}
	}
	if body == "" {
		return nil, contentError{errors.New(reasonContentEmpty)}
	}
	if content.NoLinkTracking {
		return smsMessage{body: body}, nil
	}
	unsub, _ := vars[unsubscribeURLTag].(string)
	links, err := c.m.smsShortLinks(ctx, s, messageID, unsub)
	if err != nil {
		return nil, err
	}
	if body, err = smsrender.RewriteLinks(body, func(url string) (string, error) { return links.short(ctx, url) }); err != nil {
		return nil, err
	}
	return smsMessage{body: body}, nil
}

// smsAdapter is SMS's provider adapter over the account of the routed Integration. statusCallback is
// the Integration's Ingest hook, named with every message.
type smsAdapter struct {
	sender         messaging.SMSSender
	statusCallback string
}

// DeliversOutside: an account's texts reach the carriers.
func (smsAdapter) DeliversOutside() bool { return true }

func (smsAdapter) prepare(_ context.Context, _ *ent.Scoped, content Content) (sendAs, holdreason.Reason, error) {
	if _, ok := content.(SMSContent); !ok {
		return sendAs{}, "", fmt.Errorf("outbound: %T is no SMS content", content)
	}
	return sendAs{}, "", nil
}

// send hands the text to the provider for the number. The receipt carries the provider's id of the
// message (Twilio's SID), which a delivery report names it by.
func (a smsAdapter) send(ctx context.Context, _ sendAs, msg any, to target, _ messaging.Origin) (messaging.Receipt, error) {
	m, ok := msg.(smsMessage)
	if !ok {
		return messaging.Receipt{}, fmt.Errorf("outbound: %T is no SMS message", msg)
	}
	sent, err := a.sender.Send(ctx, messaging.SMSMessage{To: to.dest, Body: m.body, StatusCallback: a.statusCallback})
	if err != nil {
		return messaging.Receipt{}, classifySMS(err)
	}
	return messaging.Receipt{MessageID: sent.ID}, nil
}

// classifySMS takes the course a refusal of the provider calls for, by whom the provider's adapter
// says it concerns (ADR 0079): the account holds the Integration (holdreason.ProviderRefused), a rate signal
// defers the send, a number that opted out at the provider is the person's stop (optedOutError, the
// send records it and Skips the message), a number that reaches no handset is suppressed, and a refusal of the number or one
// that tells nothing fails this message alone (ReasonMessageRefused), with the provider's words and
// code. A failure of the provider itself or of the network is left as it came: a retryable error.
func classifySMS(err error) error {
	e, ok := errors.AsType[*messaging.SMSError](err)
	if !ok {
		return err
	}
	switch e.Refusal {
	case messaging.SMSRefusalAccount:
		return fmt.Errorf("%w: %w", holdError{reason: holdreason.ProviderRefused}, err)
	case messaging.SMSRefusalRate:
		return deferError{wait: DeferralBackoff, err: err}
	case messaging.SMSRefusalOptedOut:
		return optedOutError{err}
	case messaging.SMSRefusalDeadNumber:
		return unreachableError{err}
	case messaging.SMSRefusalDestination, messaging.SMSRefusalAmbiguous:
		return chatRefusedError{description: ReasonMessageRefused + ": " + e.Reason()}
	default:
		return err
	}
}

// publishSent publishes sms.sent, with the Broadcast or Automation the message came from.
func (smsAdapter) publishSent(ctx context.Context, pub events.Publisher, f sentFact) error {
	e := &events.SmsSent{
		WorkspaceID:       f.workspaceID,
		ContactID:         f.req.contactID(),
		Destination:       f.dest,
		IntegrationID:     f.integrationID,
		BroadcastID:       f.req.broadcastID(),
		AutomationID:      f.req.automationID(),
		OutboundMessageID: f.messageID,
		Surface:           string(f.req.Kind()),
		SendingSource:     f.req.Source,
		DedupID:           fmt.Sprintf("%s:%d", systemevent.SmsSent, f.messageID),
	}
	if f.req.Contact != nil && f.req.Contact.Email != nil {
		e.Email = *f.req.Contact.Email
	}
	return pub.Publish(ctx, e)
}

// sendSMSTest is the Test send of an SMS Broadcast's content to the number raw, which the author
// typed: a number with its country code, else ErrTestPhoneInvalid. It goes through the Send route
// of the Integration the Broadcast names (nil: the SMS default) after the Workspace freeze, either
// holding it as ErrHeld, and skips Send-eligibility: the author names the number. It is rendered
// with sample merge data and no unsubscribe link, takes no Provider limit or Send cap, records no
// Outbound message and no sms.sent, and asks for no delivery report, which would find no message to
// land on. The text goes to the account's sender itself, past the adapter's reading of a refusal: a
// refusal of a Test send holds nothing, defers nothing and suppresses no number (a typed number that
// reaches no handset stays as it is), it is the author's ErrTestRefused with the provider's words. A
// text that does not render is emailrender.ErrInvalidContent.
func (m *Module) sendSMSTest(ctx context.Context, s *ent.Scoped, content SMSContent, raw string, integrationID *int64) (Result, error) {
	to, err := phone.Normalize(raw, "")
	if err != nil {
		return Result{}, ErrTestPhoneInvalid
	}
	g, err := m.gate(ctx, s, content, integrationID)
	if err != nil {
		return Result{}, err
	}
	if g.hold != "" {
		return Result{}, ErrHeld.With(HoldDetail(g.hold))
	}
	adapter, ok := g.route.Sender.(smsAdapter)
	if !ok {
		return Result{}, fmt.Errorf("outbound: %T is no SMS adapter", g.route.Sender)
	}
	ws, err := m.workspace(ctx, s)
	if err != nil {
		return Result{}, err
	}
	rendered, err := smsChannel{m}.render(ctx, s, Request{Content: content, Variables: testVariables(testSampleEmail)}, 0, to, ws)
	if err != nil {
		if isContentError(err) {
			return Result{}, fmt.Errorf("%w: %s", emailrender.ErrInvalidContent, err.Error())
		}
		return Result{}, err
	}
	text, ok := rendered.(smsMessage)
	if !ok {
		return Result{}, fmt.Errorf("outbound: %T is no SMS message", rendered)
	}
	if _, err := adapter.sender.Send(ctx, messaging.SMSMessage{To: to, Body: text.body}); err != nil {
		if refused, ok := errors.AsType[*messaging.SMSError](err); ok {
			return Result{}, ErrTestRefused.With(refused.Reason())
		}
		return Result{}, fmt.Errorf("outbound: sms test send: %w", err)
	}
	return Result{Outcome: Sent}, nil
}
