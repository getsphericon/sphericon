package ingest

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/phone"
	"github.com/getsphericon/sphericon/internal/preset"
)

// sms reads the texts people send an SMS Integration's sender (ADR 0076). The sender's number is
// read in E.164 (ADR 0075), with the country the provider says it is in, and a text from a number a Contact has is recorded as sms.received on
// that Contact. A number no Contact has, or one that is no phone number, records nothing and
// creates no Contact.
type sms struct{ Ignore }

// InboundMessage records a text from a Contact's number as sms.received.
func (sms) InboundMessage(ctx context.Context, tx Tx, m preset.InboundMessage) error {
	number, ok := senderNumber(m.Destination, m.Country)
	if !ok {
		return nil
	}
	c, err := tx.Scoped.Contact().Query().Where(contact.Phone(number)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lookup contact by phone: %w", err)
	}
	return tx.Pub.Publish(ctx, &events.SmsReceived{
		WorkspaceID: tx.Scoped.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email), Phone: number,
		IntegrationID: tx.Integration.ID, Text: events.ReceivedText(m.Text), DedupID: tx.DedupKey(m.ID),
	})
}

// senderNumber is the number a text came from, in E.164: a number written without its country code
// is read with country, the one the provider says the sender is in. A sender that is no number (an
// alphanumeric one) is nobody's phone.
func senderNumber(from, country string) (string, bool) {
	number, err := phone.Normalize(from, country)
	return number, err == nil
}
