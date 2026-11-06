package outbound

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// EmailContent is email's Message content (ADR 0078): a subject and a body of email-ready HTML,
// both with Liquid merge tags, and the From that overrides the Integration's configured sender.
type EmailContent struct {
	Subject string
	// Body is HTML.
	Body string
	// FromEmail and FromName override the Integration's configured sender; empty, it sends from
	// that one.
	FromEmail string
	FromName  string
}

// Channel is email.
func (EmailContent) Channel() channels.Channel { return channels.Email }

// emailChannel is email's part of the Outbound send: the renderer that adds the unsubscribe footer,
// the List-Unsubscribe URL, the open pixel and the tracked links, and the adapter over the
// Integration's EmailSender, which holds the Sending-domain gate and DKIM signing (ADR 0010, 0015).
type emailChannel struct{ m *Module }

func (c emailChannel) adapter(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (Adapter, error) {
	sender, err := c.m.senders.EmailSender(ctx, s, integ)
	if err != nil {
		return nil, err
	}
	return emailAdapter{sender: sender}, nil
}

// targets is the Request's Destination, the one address the message goes to; none for a Contact
// with no email.
func (emailChannel) targets(_ context.Context, _ *ent.Scoped, _ Request, _ Route, dest string) ([]target, error) {
	if dest == "" {
		return nil, nil
	}
	return []target{{dest: dest}}, nil
}

// unreachable: an email address is no Binding, and no email adapter reports one gone.
func (emailChannel) unreachable(context.Context, *ent.Scoped, events.Publisher, *ent.Integration, Request, target, int64) (string, error) {
	return "", nil
}

// optedOut: no email adapter reports a person's stop at send time; a complaint or an unsubscribe
// reaches the Workspace by its own path.
func (emailChannel) optedOut(context.Context, *ent.Scoped, events.Publisher, *ent.Integration, *ent.OutboundMessage, target, time.Time) error {
	return nil
}

func (c emailChannel) render(ctx context.Context, s *ent.Scoped, req Request, messageID int64, dest string, ws *ent.Workspace) (any, error) {
	msg, err := c.m.renderEmail(ctx, s, req, messageID, dest, ws)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// emailContentOf is content as email's: a Request whose content is another Channel's never
// reaches email's parts, so this is a programming error, never a recipient's failure.
func emailContentOf(content Content) (EmailContent, error) {
	email, ok := content.(EmailContent)
	if !ok {
		return EmailContent{}, fmt.Errorf("outbound: %T is no email content", content)
	}
	return email, nil
}

// renderEmail renders the content for one recipient and, for a marketing send, layers on the
// unsubscribe footer, the open/click tracking and the one-click header. A contentError is
// deterministic for this message, so the caller records it as Failed; any other error is not the
// message's and is returned to be retried. From and To are the adapter's.
func (m *Module) renderEmail(ctx context.Context, s *ent.Scoped, req Request, messageID int64, dest string, ws *ent.Workspace) (messaging.EmailMessage, error) {
	content, err := emailContentOf(req.Content)
	if err != nil {
		return messaging.EmailMessage{}, err
	}
	bindings := Bindings(req.Contact, req.Variables)
	start, err := m.startLinkBinding(ctx, s, req, messageID, content.Subject, content.Body)
	if err != nil {
		return messaging.EmailMessage{}, err
	}
	maps.Copy(bindings, start)
	email, err := emailrender.RenderEmail(content.Subject, content.Body, bindings)
	if err != nil {
		return messaging.EmailMessage{}, contentError{err}
	}
	out := messaging.EmailMessage{Subject: email.Subject, HTML: email.HTML, Text: email.Text}
	if req.Source == "" && !req.Track {
		return out, nil // Transactional, untracked: no unsubscribe, no tracking
	}
	out, err = m.layerTracking(ctx, s, req, messageID, dest, ws, out)
	if err != nil {
		return messaging.EmailMessage{}, err
	}
	// The text part says what the HTML says, tracked links and footer included (ADR 0056).
	out.Text = emailrender.FinalHTMLToText(out.HTML)
	return out, nil
}

// layerTracking adds to a rendered message what its send needs on top: for a tracked
// Transactional send the pixel and rewritten links; for a marketing send also the unsubscribe
// footer and the one-click header. Its errors are renderEmail's (see contentError).
func (m *Module) layerTracking(ctx context.Context, s *ent.Scoped, req Request, messageID int64, dest string, ws *ent.Workspace, out messaging.EmailMessage) (messaging.EmailMessage, error) {
	// One choice per message: every link and the List-Unsubscribe URL share one host.
	tr, err := m.trackerFor(ctx, s)
	if err != nil {
		return messaging.EmailMessage{}, err
	}
	if req.Source == "" {
		// A tracked Transactional send: pixel and rewritten links, no footer, no header (ADR 0053).
		html, err := tr.Instrument(out.HTML, messageID)
		if err != nil {
			return messaging.EmailMessage{}, contentError{fmt.Errorf("tracking rewrite: %w", err)}
		}
		out.HTML = html
		return out, nil
	}

	unsub := linktarget.Unsubscribe{
		Channel:     channels.Email,
		Source:      req.Source,
		Destination: dest,
		WorkspaceID: ws.ID,
		ContactID:   req.contactID(),
		BroadcastID: req.Ref.BroadcastID,
		// The link names this message, so the unsubscribe Event is about it.
		OutboundMessageID: messageID,
	}
	if req.Track {
		// Click/open tracking is layered on after rendering so re-parsing the HTML
		// cannot mangle the pixel or links; Rewrite also appends the footer.
		html, err := tr.Rewrite(out.HTML, messageID, unsub, ws.PostalAddress)
		if err != nil {
			return messaging.EmailMessage{}, contentError{fmt.Errorf("tracking rewrite: %w", err)}
		}
		out.HTML = html
	} else {
		footer, err := tr.UnsubscribeFooter(unsub, ws.PostalAddress)
		if err != nil {
			return messaging.EmailMessage{}, contentError{fmt.Errorf("unsubscribe footer: %w", err)}
		}
		out.HTML += footer
	}
	// RFC 8058 one-click header: the same source-scoped token the footer carries
	// (ADR 0012); the provider adapter covers it with the DKIM signature.
	url, err := tr.UnsubscribeURL(unsub)
	if err != nil {
		return messaging.EmailMessage{}, contentError{fmt.Errorf("unsubscribe url: %w", err)}
	}
	out.ListUnsubscribeURL = url
	return out, nil
}

// trackerFor is the Tracker a message's links are built on: the Workspace's serving Tracking domain
// when it has one (ADR 0038), else the Instance's own host.
func (m *Module) trackerFor(ctx context.Context, s *ent.Scoped) (*tracking.Tracker, error) {
	tr := m.tracker
	if m.trackingHosts != nil {
		host, err := m.trackingHosts.ServingHost(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("tracking host: %w", err)
		}
		if host != "" {
			tr = tr.OnHost(host)
		}
	}
	return tr, nil
}

// HoldUnverifiedDomain holds an email whose From is on no verified Sending domain of the Workspace
// (ADR 0015): it lifts once the domain verifies.
const HoldUnverifiedDomain = string(holdreason.UnverifiedDomain)

// emailAdapter is email's provider adapter over the EmailSender built for the Integration: SMTP and
// SES in production, a Test inbox, an in-memory one in tests.
type emailAdapter struct{ sender messaging.EmailSender }

// DeliversOutside is the EmailSender's own word (messaging.DeliversOutside).
func (a emailAdapter) DeliversOutside() bool { return messaging.DeliversOutside(a.sender) }

// prepare finds the effective From: the content's, else the Integration's configured one, the
// address that really goes on the wire, so the Sending-domain gate and stamp agree with the DKIM
// signer. Only a sender that delivers outside needs it on a verified Sending domain: nothing a
// sender that keeps its mail sends can reach anyone under a domain the Workspace does not own
// (ADR 0058).
func (a emailAdapter) prepare(ctx context.Context, s *ent.Scoped, content Content) (sendAs, string, error) {
	email, err := emailContentOf(content)
	if err != nil {
		return sendAs{}, "", err
	}
	from, name := messaging.EffectiveFrom(a.sender, email.FromEmail, email.FromName)
	if a.DeliversOutside() {
		verified, err := messaging.HasVerifiedSendingDomain(ctx, s, from)
		if err != nil {
			return sendAs{}, "", fmt.Errorf("outbound: check sending domain: %w", err)
		}
		if !verified {
			return sendAs{}, HoldUnverifiedDomain, nil
		}
	}
	return sendAs{address: from, name: name, sendingDomain: messaging.DomainOf(from)}, "", nil
}

func (a emailAdapter) send(ctx context.Context, as sendAs, msg any, to target, origin messaging.Origin) (messaging.Receipt, error) {
	email, ok := msg.(messaging.EmailMessage)
	if !ok {
		return messaging.Receipt{}, fmt.Errorf("outbound: %T is no email message", msg)
	}
	email.From, email.FromName, email.To, email.Origin = as.address, as.name, to.dest, &origin
	return sendEmail(ctx, a.sender, email)
}

// sendEmail sends one email through sender. The Sending domain losing its verification between
// prepare and the signer is a Hold, not a failure.
func sendEmail(ctx context.Context, sender messaging.EmailSender, msg messaging.EmailMessage) (messaging.Receipt, error) {
	receipt, err := sender.Send(ctx, msg)
	if errors.Is(err, messaging.ErrUnverifiedSendingDomain) {
		return messaging.Receipt{}, fmt.Errorf("%w: %w", holdError{HoldUnverifiedDomain}, err)
	}
	return receipt, err
}

// publishSent publishes email.sent and, for a provider that confirmed delivery on acceptance (a
// Test inbox), email.delivered: the Event a Delivery notice yields.
func (a emailAdapter) publishSent(ctx context.Context, pub events.Publisher, f sentFact) error {
	if err := pub.Publish(ctx, &events.EmailEngagement{
		Action:            events.NameEmailSent,
		WorkspaceID:       f.workspaceID,
		ContactID:         f.req.contactID(),
		Email:             f.dest,
		BroadcastID:       f.req.Ref.BroadcastID,
		DedupID:           fmt.Sprintf("email.sent:%d", f.messageID),
		Surface:           string(f.req.Kind),
		SendingSource:     f.req.Source,
		SendingDomain:     f.as.sendingDomain,
		AutomationID:      f.req.Ref.AutomationID,
		OutboundMessageID: f.messageID,
		ProviderMessageID: f.receipt.MessageID,
		Captured:          f.captured,
	}); err != nil {
		return err
	}
	if !f.receipt.DeliveryConfirmed {
		return nil
	}
	return pub.Publish(ctx, &events.EmailDelivery{
		Action:            events.NameEmailDelivered,
		WorkspaceID:       f.workspaceID,
		ContactID:         f.req.contactID(),
		Email:             f.dest,
		ProviderMessageID: f.receipt.MessageID,
		SendingDomain:     f.as.sendingDomain,
		DedupID:           fmt.Sprintf("%s:%d/%s", events.NameEmailDelivered, f.messageID, f.dest),
		OutboundMessageID: f.messageID,
		Surface:           string(f.req.Kind),
		SendingSource:     f.req.Source,
		BroadcastID:       f.req.Ref.BroadcastID,
		AutomationID:      f.req.Ref.AutomationID,
		Captured:          f.captured,
	})
}
