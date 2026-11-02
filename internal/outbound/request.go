package outbound

import (
	"fmt"

	"github.com/oklog/ulid/v2"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/eligibility"
)

// The three Request constructors are the only code that derives a Kind, a Source, a Ref
// and an idempotency key. The key formats are stored on every Outbound message, so they
// must not change while a send recorded under one may still be retried.

func broadcastRecipientKey(recipientID int64) string {
	return fmt.Sprintf("broadcast:%d", recipientID)
}

// BroadcastContent is the Message content of an email Broadcast: its subject, body and From.
func BroadcastContent(b *ent.Broadcast) EmailContent {
	return EmailContent{
		Subject:   b.Subject,
		Body:      b.Body,
		FromEmail: lo.FromPtr(b.FromEmail),
		FromName:  lo.FromPtr(b.FromName),
	}
}

// BroadcastRecipient is the Request for one Broadcast recipient's email. The Contact
// must have an email address.
func BroadcastRecipient(b *ent.Broadcast, rec *ent.BroadcastRecipient, c *ent.Contact) Request {
	return Request{
		Kind:          outboundmessage.KindBroadcast,
		Key:           broadcastRecipientKey(rec.ID),
		Destination:   *c.Email,
		Contact:       c,
		Source:        eligibility.SourceBroadcasts,
		Content:       BroadcastContent(b),
		Track:         true,
		Ref:           Ref{BroadcastID: b.ID, BroadcastRecipient: rec.ID},
		IntegrationID: b.IntegrationID,
	}
}

// AutomationStep is the Request for the email at index step of an Automation run, sent through the
// Integration of the version the run stands on (ADR 0065). The Contact must have an email address.
// Each Automation is its own unsubscribe Sending source; the unsubscribe click exits the
// enrollment (ADR 0001). The message binds the Event the run began on as `event` (#752).
func AutomationStep(a *ent.Automation, run *ent.AutomationRun, v *ent.AutomationVersion, step int, c *ent.Contact, content EmailContent) Request {
	return Request{
		Kind:          outboundmessage.KindAutomation,
		Key:           fmt.Sprintf("automation:%d:%d", run.ID, step),
		Destination:   *c.Email,
		Contact:       c,
		Source:        eligibility.AutomationSource(a.ID),
		Content:       content,
		Track:         true,
		Ref:           Ref{AutomationID: a.ID, AutomationRunID: run.ID, AutomationStep: &step},
		IntegrationID: v.IntegrationID,
		Variables:     EnrollmentVariables(run),
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
