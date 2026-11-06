package ingest

import (
	"context"
	"fmt"
	"strconv"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/consent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/optout"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/preset"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// sms reads the texts people send an SMS Integration's sender (ADR 0076, ADR 0077). The sender's
// number is read in E.164 (ADR 0075), with the country the provider says it is in. A text that is
// exactly a STOP keyword is the number's Channel-wide opt-out, from any number, a Contact's or not;
// one that is exactly START or UNSTOP lifts it and records the number's consent. Any other text from
// a number a Contact has is recorded as sms.received on that Contact. A number no Contact has, or
// one that is no phone number, records nothing else and creates no Contact. sphericon answers none
// of them: a STOP's or a START's confirmation is the provider's auto-reply.
//
// It also reads the delivery reports of the texts sent through the Integration (ADR 0079).
type sms struct{ Ignore }

// DeliveryNotice reads a final delivery report of a text: delivered, or undelivered with the
// provider's error code. It lands on the text the provider's id names among the Integration's, and
// moves it only forward: the first final report stamps it and records sms.delivered or
// sms.undelivered, and any later one, a redelivery included, changes nothing. A report that names no
// text yet is read again shortly (deferNotice): it may have come before the send recorded the
// provider's id, and the provider does not post it again; one that never finds a text, a text we did
// not send, records nothing. An undelivered report saying the number reaches no handset suppresses the
// number like a hard bounce, as the same answer at send time does. A report of a text whose Contact
// was erased records nothing: Erasure dropped the provider's id the report names the text by.
func (sms) DeliveryNotice(ctx context.Context, tx Tx, n preset.DeliveryNotice) error {
	result, action := outbound.ResultDelivered, events.NameSmsDelivered
	switch n.Kind {
	case preset.Delivery:
	case preset.Undelivered:
		result, action = outbound.ResultUndelivered, events.NameSmsUndelivered
	case preset.Bounce, preset.Complaint, preset.Delay, preset.Drop:
		return nil // email's kinds: a Preset that names one on sms reports no final state
	}
	msg, err := outbound.FindSMSMessage(ctx, tx.Scoped, tx.Integration.ID, n.MessageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return tx.deferNotice(ctx, n)
	}
	code := lo.Ternary(result == outbound.ResultUndelivered, n.ErrorCode, "")
	moved, err := outbound.StampFinal(ctx, tx.Scoped, msg.ID, result, code, tx.Now)
	if err != nil || !moved || msg.ContactID == nil {
		return err
	}
	if result == outbound.ResultUndelivered && deadNumber(tx.Integration.Provider, code) {
		err := suppression.Add(ctx, tx.Scoped, suppression.Entry{
			Channel: channels.SMS, Destination: msg.Destination, Reason: entsuppression.ReasonBounce,
			ContactID: *msg.ContactID,
		})
		if err != nil {
			return fmt.Errorf("ingest: suppress a dead number: %w", err)
		}
	}
	c, err := tx.Scoped.Contact().Get(ctx, *msg.ContactID)
	if err != nil {
		return fmt.Errorf("ingest: the contact of sms message %d: %w", msg.ID, err)
	}
	return tx.Pub.Publish(ctx, &events.SmsDelivery{
		Action: action, WorkspaceID: tx.Scoped.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email),
		IntegrationID: tx.Integration.ID, Destination: msg.Destination, ErrorCode: code,
		BroadcastID: lo.FromPtr(msg.BroadcastID), AutomationID: lo.FromPtr(msg.AutomationID), OutboundMessageID: msg.ID,
		Surface: string(msg.Kind), SendingSource: lo.FromPtr(msg.SendingSource), DedupID: tx.DedupKey(n.ID),
	})
}

// deadNumber says whether the provider's error code of an undelivered report means the number
// reaches no handset (Twilio's 30005, 30006), read the way the provider's adapter reads the same
// code at send time.
func deadNumber(provider providers.Provider, code string) bool {
	n, err := strconv.Atoi(code)
	if err != nil || provider != providers.Twilio {
		return false
	}
	return twilio.Classify(0, n, "") == messaging.SMSRefusalDeadNumber
}

// InboundMessage reads a text for what it means: a stop, a start, or a reply.
func (sms) InboundMessage(ctx context.Context, tx Tx, m preset.InboundMessage) error {
	number, ok := senderNumber(m.Destination, m.Country)
	if !ok {
		return nil
	}
	switch {
	case optout.IsStopKeyword(m.Text):
		return optout.StopSMS(ctx, tx.Scoped, number, unsubscribe.ViaKeyword)
	case optout.IsStartKeyword(m.Text):
		return consent.StartSMSIn(ctx, tx.Scoped, tx.Pub, number)
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
