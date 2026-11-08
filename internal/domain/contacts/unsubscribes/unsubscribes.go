// Package unsubscribes is the one place an Unsubscribe row is written (ADR 0001, ADR 0102): a
// per-(Channel, Destination, Sending source) opt-out, keyed by the Destination so it outlives the
// Contact. The modules that decide an opt-out (optout for a stop in the Channel, the unsubscribe
// use case for a link or an API call) record it through these functions; each runs inside the
// transaction s is bound to.
package unsubscribes

import (
	"context"
	"fmt"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var (
	// ErrDestinationEmpty: the destination is blank after normalization.
	ErrDestinationEmpty = validation.RefuseField("destination", "consent: destination must not be empty", i18n.ErrorsDestinationEmpty)
	// ErrDestinationInvalid: the destination is not what the Channel's Destinations are.
	ErrDestinationInvalid = suppressions.ErrDestinationInvalid
)

// SMSNumber is a destination on sms: the phone number in E.164, which must carry its country code
// (a Confirmation or an opt-out has no Contact country to read a local number with).
func SMSNumber(destination string) (string, error) {
	if strings.TrimSpace(destination) == "" {
		return "", ErrDestinationEmpty
	}
	number, err := phone.Normalize(destination, "")
	if err != nil {
		return "", ErrDestinationInvalid
	}
	return number, nil
}

// OptOut is one Unsubscribe to record.
type OptOut struct {
	Channel     channels.Channel
	Destination string
	Source      string
	// ContactID is the Contact the Destination belongs to, 0 when none does.
	ContactID int64
	// Via is how the person stopped, "" when the row does not say (on email the Event does).
	Via unsubscribe.Via
}

// The unique constraint of the Unsubscribe's table (ADR 0106): one opt-out per (Channel,
// Destination, Sending source), which Add and AddSMS insert ON CONFLICT DO NOTHING and EraseSMS
// rekeys to a hash only where no opt-out holds it.
var _ = db.Declare(
	db.Fault("unsubscribes_ws_channel_dest_source",
		"Add and AddSMS insert ON CONFLICT (workspace_id, channel, destination, sending_source) DO NOTHING, and EraseSMS rekeys a number to its hash under the Contact's Erasure only where no opt-out holds the hash"),
)

// Add writes o unless it stands already, and reports whether it wrote it: a repeated opt-out
// (a second click, a concurrent request) changes nothing. A Contact gone by now (erased between a
// send and the click) leaves the opt-out unlinked: it is keyed by its Destination.
func Add(ctx context.Context, s *ent.Scoped, o OptOut) (bool, error) {
	create := s.Unsubscribe().Create().
		SetChannel(o.Channel).
		SetDestination(o.Destination).
		SetSendingSource(o.Source)
	contactID, err := contactlock.Referenced(ctx, s, o.ContactID)
	if err != nil {
		return false, fmt.Errorf("lock the opt-out's contact: %w", err)
	}
	if contactID != 0 {
		create.SetContactID(contactID)
	}
	if o.Via != "" {
		create.SetVia(o.Via)
	}
	return insert(ctx, create)
}

// insert writes create ON CONFLICT on the Unsubscribe's key DO NOTHING, and reports whether it
// wrote a row.
func insert(ctx context.Context, create *ent.UnsubscribeScopedCreate) (bool, error) {
	err := create.
		OnConflictColumns(unsubscribe.FieldWorkspaceID, unsubscribe.FieldChannel, unsubscribe.FieldDestination, unsubscribe.FieldSendingSource).
		DoNothing().
		Exec(ctx)
	added, err := db.Inserted(err)
	if err != nil {
		return false, fmt.Errorf("record opt-out: %w", err)
	}
	return added, nil
}

// Get is the opt-out of destination on channel from source.
func Get(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination, source string) (*ent.Unsubscribe, error) {
	return s.Unsubscribe().Query().Where(
		unsubscribe.ChannelEQ(channel),
		unsubscribe.DestinationEQ(destination),
		unsubscribe.SendingSourceEQ(source),
	).Only(ctx)
}

// AddSMS records the Channel-wide opt-out of a phone number (scope `everything`, Channel `sms`, the
// number in E.164), naming the Contact whose phone it is, unless it stands already (by the number
// or by its hash), and returns the row it wrote, nil when it wrote none: a number already opted out
// changes nothing, and keeps the way it first stopped. via is how the person stopped, which the
// Contact card shows.
func AddSMS(ctx context.Context, s *ent.Scoped, number string, via unsubscribe.Via) (*ent.Unsubscribe, error) {
	// The opt-out of an erased person is kept by the hash of the number (EraseSMS): a second row by
	// the number itself would not conflict with it.
	hashed, err := s.Unsubscribe().Query().Where(
		unsubscribe.ChannelEQ(channels.SMS),
		unsubscribe.DestinationEQ(phone.Hash(number)),
		unsubscribe.SendingSourceEQ(eligibility.SourceEverything),
	).Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("find opt-out: %w", err)
	}
	if hashed {
		return nil, nil
	}
	create := s.Unsubscribe().Create().
		SetChannel(channels.SMS).
		SetDestination(number).
		SetSendingSource(eligibility.SourceEverything).
		SetVia(via)
	holder, err := s.Contact().Query().Where(contact.Phone(number)).OnlyID(ctx)
	switch {
	case err == nil:
		create.SetContactID(holder)
	case !ent.IsNotFound(err):
		return nil, fmt.Errorf("find the number's contact: %w", err)
	}
	added, err := insert(ctx, create)
	if err != nil || !added {
		return nil, err
	}
	return Get(ctx, s, channels.SMS, number, eligibility.SourceEverything)
}

// LiftSMS lifts the Channel-wide opt-out of a phone number: the person texted START. Nothing to
// lift is a no-op. It records no consent: the START's Confirmation is the caller's to record.
func LiftSMS(ctx context.Context, s *ent.Scoped, number string) error {
	if _, err := s.Unsubscribe().Delete().Where(smsEverything(number)...).Exec(ctx); err != nil {
		return fmt.Errorf("lift opt-out: %w", err)
	}
	return nil
}

// SMSOptOut is the Channel-wide opt-out of a phone number, nil when it has none.
func SMSOptOut(ctx context.Context, s *ent.Scoped, number string) (*ent.Unsubscribe, error) {
	u, err := s.Unsubscribe().Query().Where(smsEverything(number)...).Order(ent.Asc(unsubscribe.FieldID)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find opt-out: %w", err)
	}
	return u, nil
}

// smsEverything is the Channel-wide opt-out of the number: kept by the number, or by its hash once
// the Contact whose phone it was is erased (EraseSMS), so a STOP, a START and the Contact card find
// an erased person's opt-out when the number comes back.
func smsEverything(number string) []predicate.Unsubscribe {
	return []predicate.Unsubscribe{
		unsubscribe.ChannelEQ(channels.SMS),
		unsubscribe.DestinationIn(number, phone.Hash(number)),
		unsubscribe.SendingSourceEQ(eligibility.SourceEverything),
	}
}

// LiftTelegram lifts the Channel-wide opt-out of a Telegram user: they asked to receive again, by
// unblocking a bot or by starting one. Nothing to lift is a no-op. Only the `everything` scope is
// lifted: a source-scoped opt-out the person chose by a link in a message stays.
func LiftTelegram(ctx context.Context, s *ent.Scoped, destination string) error {
	if _, err := s.Unsubscribe().Delete().Where(
		unsubscribe.ChannelEQ(channels.Telegram),
		unsubscribe.DestinationEQ(destination),
		unsubscribe.SendingSourceEQ(eligibility.SourceEverything),
	).Exec(ctx); err != nil {
		return fmt.Errorf("lift opt-out: %w", err)
	}
	return nil
}
