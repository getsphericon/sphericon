package outbound

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/smsrender"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// SMSContent is SMS's Message content (ADR 0078): one plain text with Liquid merge tags, no media,
// nothing appended.
type SMSContent struct {
	Text string
}

// Channel is SMS.
func (SMSContent) Channel() messaging.Channel { return messaging.ChannelSMS }

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
func (smsChannel) unreachable(ctx context.Context, s *ent.Scoped, _ events.Publisher, _ *ent.Integration, req Request, to target, _ int64) (string, error) {
	err := suppression.Add(ctx, s, suppression.Entry{
		Channel: entsuppression.ChannelSms, Destination: to.dest, Reason: entsuppression.ReasonBounce, ContactID: req.contactID(),
	})
	if err != nil {
		return "", fmt.Errorf("outbound: suppress a dead number: %w", err)
	}
	return eligibility.ReasonSuppressed, nil
}

// smsMessage is a rendered text, as the provider takes it.
type smsMessage struct{ body string }

// render renders the text for one recipient: Liquid with the Contact's and the Enrollment's merge
// tags, and `unsubscribe_url` when the text uses it. A text that does not render, or renders to
// nothing, is the message's own failure, the same on every attempt.
func (c smsChannel) render(ctx context.Context, s *ent.Scoped, req Request, _ int64, dest string, ws *ent.Workspace) (any, error) {
	content, ok := req.Content.(SMSContent)
	if !ok {
		return nil, fmt.Errorf("outbound: %T is no SMS content", req.Content)
	}
	vars := Bindings(req.Contact, req.Variables)
	if err := c.m.bindUnsubscribeURL(ctx, s, req, dest, ws, messaging.ChannelSMS, content.Text, vars); err != nil {
		return nil, err
	}
	body, err := smsrender.Render(content.Text, vars)
	if err != nil {
		return nil, contentError{err}
	}
	if body == "" {
		return nil, contentError{errors.New(reasonContentEmpty)}
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

func (smsAdapter) prepare(_ context.Context, _ *ent.Scoped, content Content) (sendAs, string, error) {
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
// says it concerns (ADR 0079): the account holds the Integration (HoldProviderRefused), a rate signal
// defers the send, a number that reaches no handset is suppressed, and a refusal of the number or one
// that tells nothing fails this message alone (ReasonMessageRefused), with the provider's words and
// code. A failure of the provider itself or of the network is left as it came: a retryable error.
func classifySMS(err error) error {
	e, ok := errors.AsType[*messaging.SMSError](err)
	if !ok {
		return err
	}
	switch e.Refusal {
	case messaging.SMSRefusalAccount:
		return fmt.Errorf("%w: %w", holdError{reason: HoldProviderRefused}, err)
	case messaging.SMSRefusalRate:
		return deferError{wait: DeferralBackoff, err: err}
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
		BroadcastID:       f.req.Ref.BroadcastID,
		AutomationID:      f.req.Ref.AutomationID,
		OutboundMessageID: f.messageID,
		Surface:           string(f.req.Kind),
		SendingSource:     f.req.Source,
		DedupID:           fmt.Sprintf("%s:%d", events.NameSmsSent, f.messageID),
	}
	if f.req.Contact != nil && f.req.Contact.Email != nil {
		e.Email = *f.req.Contact.Email
	}
	return pub.Publish(ctx, e)
}
