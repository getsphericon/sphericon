// Package suppressions is the one place Suppression rows are written, listed and removed
// (ADR 0015, GLOSSARY "Suppression"): the hard, global do-not-send registry keyed by
// (channel, destination). The suppression job, the manual add and the site handlers all
// go through it, so the "insert, ignore on conflict" statement exists once. Unsubscribe
// and confirm stay in consent; eligibility reads Suppression with its own SQL.
package suppressions

import (
	"context"
	"net/mail"
	"regexp"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
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

// The unique constraint of the Suppression's table (ADR 0106): one active entry per (Channel,
// Destination).
var _ = db.Declare(
	db.Fault("suppressions_workspace_id_channel_destination",
		"Add inserts ON CONFLICT on the active entry's key, and the Erasure rekeys a number to its hash only where no active entry holds the hash"),
)

// Add suppresses a destination in the scoped Workspace with one insert. It is
// idempotent per (channel, destination): an existing active entry keeps its reason. A
// lifted entry does not count (ADR 0049): the destination is suppressed anew in a new row.
func Add(ctx context.Context, s *ent.Scoped, e Entry) error {
	_, err := add(ctx, s, e)
	return err
}

func add(ctx context.Context, s *ent.Scoped, e Entry) (bool, error) {
	dest := emailcase.Of(e.Destination)
	if dest == "" {
		return false, ErrDestinationEmpty
	}
	channel := e.Channel
	if channel == "" {
		channel = channels.Email
	}
	if !ValidDestination(string(channel), dest) {
		return false, ErrDestinationInvalid
	}
	create := s.Suppression().Create().
		SetChannel(channel).
		SetDestination(dest).
		SetReason(e.Reason)
	if e.ContactID != 0 {
		create.SetContactID(e.ContactID)
	}
	err := create.
		OnConflictColumnsWhere(entsql.IsNull(entsuppression.FieldLiftedAt),
			entsuppression.FieldWorkspaceID, entsuppression.FieldChannel, entsuppression.FieldDestination).
		DoNothing().
		Exec(ctx)
	added, err := db.Inserted(err)
	if err != nil {
		return false, err
	}
	return added, nil
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
		if addr, err := mail.ParseAddress(emailcase.Of(destination)); err != nil || addr.Address != emailcase.Of(destination) {
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
		Where(entsuppression.ChannelEQ(channel), entsuppression.DestinationEQ(emailcase.Of(destination)), entsuppression.LiftedAtIsNil()).
		Only(ctx)
}

// List returns one page of the Workspace's active Suppressions, newest first, or of its
// lifted ones when lifted is true.
func List(ctx context.Context, s *ent.Scoped, p pagination.Params, lifted bool) (pagination.Page[*ent.Suppression], error) {
	return pagination.List(ctx, p, query(s, lifted).Order(ent.Desc(entsuppression.FieldID)))
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
// Suppression the Erasure keyed by its hash (suppressionsunchecked.EraseContact), which refuses the number as much:
// lifting the number's lifts that twin too, so the number can be texted again.
func Remove(ctx context.Context, s *ent.Scoped, id int64, at time.Time) error {
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
		SetLiftedAt(at).
		Save(ctx)
	return err
}
