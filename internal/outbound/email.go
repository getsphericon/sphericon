package outbound

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/messaging"
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
func (EmailContent) Channel() messaging.Channel { return messaging.ChannelEmail }

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

func (c emailChannel) render(ctx context.Context, s *ent.Scoped, req Request, messageID int64, dest string, ws *ent.Workspace) (any, error) {
	content, err := emailContentOf(req.Content)
	if err != nil {
		return nil, err
	}
	return c.m.renderEmail(ctx, s, req, content, messageID, dest, ws)
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
func (m *Module) renderEmail(ctx context.Context, s *ent.Scoped, req Request, content EmailContent, messageID int64, dest string, ws *ent.Workspace) (messaging.EmailMessage, error) {
	email, err := emailrender.RenderEmail(content.Subject, content.Body, Bindings(req.Contact, req.Variables))
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
	tr := m.tracker
	if m.trackingHosts != nil {
		host, err := m.trackingHosts.ServingHost(ctx, s)
		if err != nil {
			return messaging.EmailMessage{}, fmt.Errorf("tracking host: %w", err)
		}
		if host != "" {
			tr = tr.OnHost(host)
		}
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

	unsub := tracking.UnsubTarget{
		Source:      req.Source,
		Destination: dest,
		WorkspaceID: ws.ID,
		ContactID:   req.contactID(),
		BroadcastID: req.Ref.BroadcastID,
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
	from, name := effectiveFrom(a.sender, email.FromEmail, email.FromName)
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

func (a emailAdapter) send(ctx context.Context, as sendAs, msg any, dest string, origin messaging.Origin) (messaging.Receipt, error) {
	email, ok := msg.(messaging.EmailMessage)
	if !ok {
		return messaging.Receipt{}, fmt.Errorf("outbound: %T is no email message", msg)
	}
	email.From, email.FromName, email.To, email.Origin = as.address, as.name, dest, &origin
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
		ContactID:         f.contactID,
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
		ContactID:         f.contactID,
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

// effectiveFrom is the From the message goes out with: the given one, else the sender's
// configured default.
func effectiveFrom(sender messaging.EmailSender, from, name string) (address, displayName string) {
	if d, ok := sender.(messaging.DefaultFromer); ok {
		df, dn := d.DefaultFrom()
		return cmp.Or(from, df), cmp.Or(name, dn)
	}
	return from, name
}
