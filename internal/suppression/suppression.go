// Package suppression is the one place Suppression rows are written, listed and removed
// (ADR 0015, GLOSSARY "Suppression"): the hard, global do-not-send registry keyed by
// (channel, destination). The suppression job, the manual add and the site handlers all
// go through it, so the "insert, ignore on conflict" statement exists once. Unsubscribe
// and confirm stay in consent; eligibility reads Suppression with its own SQL.
package suppression

import (
	"context"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var (
	// ErrDestinationEmpty: the destination is blank after normalization. Every surface answers
	// it alike, on the `destination` field (review rule 15).
	ErrDestinationEmpty = validation.RefuseField("destination", "suppression: destination must not be empty", i18n.ErrorsDestinationInvalid)
	// ErrDestinationInvalid: the destination is not what the Channel's Destinations are.
	ErrDestinationInvalid = validation.RefuseField("destination", "suppression: destination is not valid for the channel", i18n.ErrorsDestinationInvalid)
	// ErrNotFound: no such Suppression in this Workspace.
	ErrNotFound = validation.NotFound("suppression: not found", i18n.ErrorsSuppressionNotFound)
)

// NormalizeDestination is the single definition of "normalized destination":
// lower-cased and trimmed. Suppression and Unsubscribe rows store destinations this
// way, and every lookup must apply it so the readers of those rows agree on one form.
func NormalizeDestination(dest string) string {
	return strings.ToLower(strings.TrimSpace(dest))
}

// ForEvent is the rule that decides whether a delivery event suppresses its address, and
// why: a complaint (events.NameEmailComplained) always does, a bounce (events.NameEmailBounced)
// only when hard (bounceKind events.BounceKindHard; a soft one, such as a full mailbox, is
// temporary), anything else never. Unsubscribe is not a Suppression (ADR 0001).
func ForEvent(name, bounceKind string) (entsuppression.Reason, bool) {
	switch name {
	case events.NameEmailComplained:
		return entsuppression.ReasonComplaint, true
	case events.NameEmailBounced:
		if bounceKind == events.BounceKindHard {
			return entsuppression.ReasonBounce, true
		}
		return "", false
	default:
		return "", false
	}
}

// Entry is one Suppression to add. ContactID is the Contact the destination belongs
// to, when known (display only); zero means none. Channel is email when empty.
type Entry struct {
	Channel     channels.Channel
	Destination string
	Reason      entsuppression.Reason
	ContactID   int64
}

// telegramUserID is what a Telegram Destination is: the user's numeric id.
var telegramUserID = regexp.MustCompile(`^\d{1,20}$`)

// e164 is what an SMS Destination is: a phone number in E.164.
var e164 = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

// ValidDestination reports whether dest, already normalized, can be a Destination on the Channel:
// anything non-empty on email (the contract checks the address), a Telegram user id on Telegram, a
// number in E.164 on SMS.
func ValidDestination(channel, dest string) bool {
	switch channel {
	case string(channels.Telegram):
		return telegramUserID.MatchString(dest)
	case string(channels.SMS):
		return e164.MatchString(dest)
	}
	return dest != ""
}

// Add suppresses a destination in the scoped Workspace with one insert. It is
// idempotent per (channel, destination): an existing active entry keeps its reason. A
// lifted entry does not count (ADR 0049): the destination is suppressed anew in a new row.
func Add(ctx context.Context, s *ent.Scoped, e Entry) error {
	dest := NormalizeDestination(e.Destination)
	if dest == "" {
		return ErrDestinationEmpty
	}
	channel := e.Channel
	if channel == "" {
		channel = channels.Email
	}
	if !ValidDestination(string(channel), dest) {
		return ErrDestinationInvalid
	}
	create := s.Suppression().Create().
		SetChannel(channel).
		SetDestination(dest).
		SetReason(e.Reason)
	if e.ContactID != 0 {
		create.SetContactID(e.ContactID)
	}
	return create.
		OnConflictColumnsWhere(sql.IsNull(entsuppression.FieldLiftedAt),
			entsuppression.FieldWorkspaceID, entsuppression.FieldChannel, entsuppression.FieldDestination).
		Ignore().
		Exec(ctx)
}

// AddManual is Add with the manual reason (an Owner's or an API caller's request), and
// returns the entry, the existing one when the destination was already suppressed.
func AddManual(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (*ent.Suppression, error) {
	if channel == "" {
		channel = channels.Email
	}
	// Someone typed it: the address must be one (the suppression job's addresses come from
	// providers), and a number is stored in E.164, read with the country code it must carry.
	switch channel {
	case channels.Email:
		if addr, err := mail.ParseAddress(NormalizeDestination(destination)); err != nil || addr.Address != NormalizeDestination(destination) {
			return nil, ErrDestinationInvalid
		}
	case channels.SMS:
		if strings.TrimSpace(destination) == "" {
			return nil, ErrDestinationEmpty
		}
		number, err := phone.Normalize(destination, "")
		if err != nil {
			return nil, ErrDestinationInvalid
		}
		destination = number
	case channels.Telegram:
		// A user id is checked by Add.
	}
	if err := Add(ctx, s, Entry{Channel: channel, Destination: destination, Reason: entsuppression.ReasonManual}); err != nil {
		return nil, err
	}
	return s.Suppression().Query().
		Where(entsuppression.ChannelEQ(channel), entsuppression.DestinationEQ(NormalizeDestination(destination)), entsuppression.LiftedAtIsNil()).
		Only(ctx)
}

// List returns one page of the Workspace's active Suppressions, newest first, or of its
// lifted ones when lifted is true.
func List(ctx context.Context, s *ent.Scoped, p pagination.Params, lifted bool) (pagination.Page[*ent.Suppression], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return query(s, lifted).Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Suppression, error) {
			return query(s, lifted).
				Order(ent.Desc(entsuppression.FieldID)).
				Limit(limit).
				Offset(offset).
				All(ctx)
		})
}

// query is the Workspace's active Suppressions, or its lifted ones.
func query(s *ent.Scoped, lifted bool) *ent.SuppressionQuery {
	if lifted {
		return s.Suppression().Query().Where(entsuppression.LiftedAtNotNil())
	}
	return s.Suppression().Query().Where(entsuppression.LiftedAtIsNil())
}

// Remove lifts an active Suppression of the scoped Workspace by id: the row stays with its
// lift time and the destination can receive mail again. A missing or already lifted one is
// ErrNotFound. On sms a number suppressed again after its Contact's Erasure also has the
// Suppression the Erasure keyed by its hash (EraseContact), which refuses the number as much:
// lifting the number's lifts that twin too, so the number can be texted again.
func Remove(ctx context.Context, s *ent.Scoped, id int64) error {
	row, err := s.Suppression().Query().Where(entsuppression.ID(id), entsuppression.LiftedAtIsNil()).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	lifted := entsuppression.ID(row.ID)
	if row.Channel == channels.SMS && e164.MatchString(row.Destination) {
		lifted = entsuppression.Or(lifted, entsuppression.And(
			entsuppression.ChannelEQ(channels.SMS), entsuppression.DestinationEQ(phone.Hash(row.Destination))))
	}
	_, err = s.Suppression().Update().
		Where(lifted, entsuppression.LiftedAtIsNil()).
		SetLiftedAt(time.Now()).
		Save(ctx)
	return err
}

// EraseContact is the Suppression part of the Contact's Erasure: its lifted rows are
// deleted, since a lifted one refuses nothing any more, and its active rows only lose the
// Contact reference, because they are keyed by destination and keep refusing the address.
// On sms a number leaves only its hash (ADR 0021): the rows of the Contact's phone, number ("" when
// it has none), count as its own, and an active one keeps refusing by phone.Hash of the number. A
// hashed one already standing (a person erased before with the number) makes it redundant.
func EraseContact(ctx context.Context, s *ent.Scoped, contactID int64, number string) error {
	mine := entsuppression.ContactID(contactID)
	if number != "" {
		mine = entsuppression.Or(mine, entsuppression.And(
			entsuppression.ChannelEQ(channels.SMS), entsuppression.DestinationEQ(number)))
	}
	if _, err := s.Suppression().Delete().Where(mine, entsuppression.LiftedAtNotNil()).Exec(ctx); err != nil {
		return err
	}
	if err := hashNumbers(ctx, s, mine); err != nil {
		return err
	}
	_, err := s.Suppression().Update().Where(entsuppression.ContactID(contactID)).ClearContactID().Save(ctx)
	return err
}

// hashNumbers rekeys the active sms Suppressions mine matches by the hash of their number.
func hashNumbers(ctx context.Context, s *ent.Scoped, mine predicate.Suppression) error {
	rows, err := s.Suppression().Query().
		Where(mine, entsuppression.ChannelEQ(channels.SMS), entsuppression.LiftedAtIsNil()).
		All(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		hashed := phone.Hash(r.Destination)
		twin, err := s.Suppression().Query().Where(
			entsuppression.ChannelEQ(channels.SMS),
			entsuppression.DestinationEQ(hashed),
			entsuppression.LiftedAtIsNil(),
		).Exist(ctx)
		if err != nil {
			return err
		}
		if twin {
			err = s.Suppression().DeleteOneID(r.ID).Exec(ctx)
		} else {
			err = s.Suppression().UpdateOneID(r.ID).SetDestination(hashed).Exec(ctx)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
