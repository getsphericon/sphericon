package outbound

import (
	"fmt"

	"github.com/oklog/ulid/v2"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// The three Request constructors are the only code that derives a Kind, a Source, a Ref
// and an idempotency key. The key formats are stored on every Outbound message, so they
// must not change while a send recorded under one may still be retried.

func broadcastRecipientKey(recipientID int64) string {
	return fmt.Sprintf("broadcast:%d", recipientID)
}

// BroadcastContent is the Message content of a Broadcast, in the shape of its Channel (ADR 0078,
// 0080): on email its subject, body and From, on Telegram its text, image, buttons, link preview and unsubscribe button.
func BroadcastContent(b *ent.Broadcast) (Content, error) {
	switch b.Channel {
	case broadcast.ChannelEmail:
		return EmailContent{
			Subject:   lo.FromPtr(b.Subject),
			Body:      lo.FromPtr(b.Body),
			FromEmail: lo.FromPtr(b.FromEmail),
			FromName:  lo.FromPtr(b.FromName),
		}, nil
	case broadcast.ChannelTelegram:
		return TelegramContent{
			Text:              lo.FromPtr(b.Text),
			ImageURL:          lo.FromPtr(b.ImageURL),
			Buttons:           b.Buttons,
			NoLinkPreview:     !b.LinkPreview,
			UnsubscribeButton: lo.FromPtr(b.UnsubscribeButton),
		}, nil
	default:
		return nil, fmt.Errorf("outbound: broadcast %d is on channel %q, which has no content", b.ID, b.Channel)
	}
}

// BroadcastRecipient is the Request for one Broadcast recipient's message on the Broadcast's
// Channel. Its Destination is the Contact's on that Channel: on email the address, which a Contact
// with none is Skipped for; a Channel of Bindings reads them after the claim.
func BroadcastRecipient(b *ent.Broadcast, rec *ent.BroadcastRecipient, c *ent.Contact) (Request, error) {
	content, err := BroadcastContent(b)
	if err != nil {
		return Request{}, err
	}
	return Request{
		Kind:          outboundmessage.KindBroadcast,
		Key:           broadcastRecipientKey(rec.ID),
		Destination:   contactDestination(content.Channel(), c),
		Contact:       c,
		Source:        eligibility.SourceBroadcasts,
		Content:       content,
		Track:         true,
		Ref:           Ref{BroadcastID: b.ID, BroadcastRecipient: rec.ID},
		IntegrationID: b.IntegrationID,
	}, nil
}

// AutomationStep is the Request for the send step at index step of an Automation run: its content,
// on the content's Channel, through the Integration the step names (nil: that Channel's default,
// ADR 0081). The Destination is the Contact's on that Channel; a Contact with none is Skipped
// with eligibility.ReasonNoDestination, recorded for the step like any other skip. Each Automation
// is its own unsubscribe Sending source; the unsubscribe click exits the enrollment (ADR 0001). The
// message binds the Event the run began on as `event` (#752).
func AutomationStep(a *ent.Automation, run *ent.AutomationRun, step int, integrationID *int64, c *ent.Contact, content Content) Request {
	return Request{
		Kind:          outboundmessage.KindAutomation,
		Key:           fmt.Sprintf("automation:%d:%d", run.ID, step),
		Destination:   contactDestination(content.Channel(), c),
		Contact:       c,
		Source:        eligibility.AutomationSource(a.ID),
		Content:       content,
		Track:         true,
		Ref:           Ref{AutomationID: a.ID, AutomationRunID: run.ID, AutomationStep: &step},
		IntegrationID: integrationID,
		Variables:     EnrollmentVariables(run),
	}
}

// contactDestination is the Contact's Destination on the Channel, "" when it has none: on email its
// address, on SMS its phone (stored in E.164, ADR 0075).
func contactDestination(ch messaging.Channel, c *ent.Contact) string {
	switch ch {
	case messaging.ChannelEmail:
		return lo.FromPtr(c.Email)
	case messaging.ChannelSMS:
		return lo.FromPtr(c.Phone)
	default:
		return ""
	}
}

// TransactionalMessage is what a Transactional send is made of. ClientKey is the
// caller's own idempotency token: a repeated one replays the recorded send. Empty, the
// send is its own every time.
type TransactionalMessage struct {
	ClientKey   string
	Destination string
	ContactID   int64
	TemplateID  int64
	// Content is the Template's current content.
	Content   EmailContent
	Variables map[string]any
	// Track opts the send into open and click tracking; it stays Transactional (ADR 0053).
	Track bool
}

// Transactional is the Request for a Transactional message (no unsubscribe Source).
func Transactional(m TransactionalMessage) Request {
	key := "transactional:auto:" + ulid.Make().String()
	if m.ClientKey != "" {
		key = "transactional:" + m.ClientKey
	}
	return Request{
		Kind:        outboundmessage.KindTransactional,
		Key:         key,
		Destination: m.Destination,
		ContactID:   m.ContactID,
		Content:     m.Content,
		Variables:   m.Variables,
		Track:       m.Track,
		Ref:         Ref{TemplateID: m.TemplateID},
	}
}
