package outbound

import (
	"context"
	"fmt"

	"github.com/oklog/ulid/v2"

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

// BroadcastRecipient is the Request for one Broadcast recipient's email. The Contact
// must have an email address.
func BroadcastRecipient(b *ent.Broadcast, rec *ent.BroadcastRecipient, c *ent.Contact) Request {
	req := Request{
		Kind:        outboundmessage.KindBroadcast,
		Key:         broadcastRecipientKey(rec.ID),
		Destination: *c.Email,
		Contact:     c,
		Source:      eligibility.SourceBroadcasts,
		Subject:     b.Subject,
		Body:        b.Body,
		TrackID:     rec.ID,
		Ref:         Ref{BroadcastID: b.ID, BroadcastRecipient: rec.ID},
	}
	if b.FromEmail != nil {
		req.FromEmail = *b.FromEmail
	}
	if b.FromName != nil {
		req.FromName = *b.FromName
	}
	return req
}

// AutomationStep is the Request for the email at index step of an Automation run. The
// Contact must have an email address. Each Automation is its own unsubscribe Sending
// source; the unsubscribe click exits the enrollment (ADR 0001).
func AutomationStep(a *ent.Automation, run *ent.AutomationRun, step int, c *ent.Contact, subject, body string) Request {
	return Request{
		Kind:        outboundmessage.KindAutomation,
		Key:         fmt.Sprintf("automation:%d:%d", run.ID, step),
		Destination: *c.Email,
		Contact:     c,
		Source:      eligibility.AutomationSource(a.ID),
		Subject:     subject,
		Body:        body,
		Ref:         Ref{AutomationID: a.ID, AutomationRunID: run.ID, AutomationStep: &step},
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
	Subject     string
	Body        string
	Variables   map[string]any
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
		Subject:     m.Subject,
		Body:        m.Body,
		Variables:   m.Variables,
		Ref:         Ref{TemplateID: m.TemplateID},
	}
}

// MarkRecipientFailed gives up the pending claim of a Broadcast recipient's send for
// good, recording cause (see MarkFailed).
func (m *Module) MarkRecipientFailed(ctx context.Context, s *ent.Scoped, recipientID int64, cause error) error {
	return m.MarkFailed(ctx, s, Request{Key: broadcastRecipientKey(recipientID)}, cause)
}
