// Package consentstamps orders the Consent observations of a Destination (ADR 0154): it keeps when
// the observation that set each consent state occurred (its stamp), and lets a new observation
// change that state only when it occurred later. The owners of the states (unsubscribes,
// confirmations) ask it before they write, after their provenance checks: time orders only the
// observations allowed to replace each other. It and its Erasure step (consentstampsunchecked) are
// the one place a ConsentStamp row is written, and each function runs inside the transaction s is
// bound to.
package consentstamps

import (
	"context"
	"errors"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentstamp"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

// ErrNoOccurrence refuses an observation applied without the time it occurred, a caller's bug.
var ErrNoOccurrence = errors.New("consent: an observation needs the time it occurred")

// Occurrence is when a Consent observation occurred: the provider's time where it gives one, else
// when the Instance received it, or the time an import states. An observation a bot numbered (a
// Telegram update) also carries the bot and its number, which order two observations of that bot.
type Occurrence struct {
	At time.Time
	// IntegrationID is the Integration whose sequence numbered the observation, 0 when none did.
	IntegrationID int64
	// Sequence is the observation's number in that sequence (Telegram's update_id).
	Sequence int64
}

// At is an Occurrence known only by its time.
func At(t time.Time) Occurrence { return Occurrence{At: t} }

// Fact is the consent state an observation sets.
type Fact int

const (
	// OptOut is the opt-out state of a (Channel, Destination, Sending source): an Unsubscribe, or
	// the return that lifted it.
	OptOut Fact = iota
	// Confirmation is the Confirmation state of a (Channel, Destination): a Confirmation, or the
	// opt-out of everything that withdrew it. It is stamped on the `everything` source.
	Confirmation
)

// Key is what a consent state belongs to.
type Key struct {
	Channel     channels.Channel
	Destination string
	Source      string
}

// The unique constraint of the stamp's table: one row per key, which Hold inserts ON CONFLICT DO
// NOTHING before it locks it.
var _ = db.Declare(
	db.Fault("consent_stamps_ws_channel_dest_source", "Hold inserts ON CONFLICT (workspace_id, channel, destination, sending_source) DO NOTHING"),
)

// Hold locks the stamp row of k to the end of the transaction, making it if there is none, and
// returns it: the observations of one key are handled one at a time, so a comparison and the write
// it allows are one step. An owner holds it before it reads the state it decides on, and before
// the Contact's lock (contactlock), which its write may take.
func Hold(ctx context.Context, s *ent.Scoped, k Key) (*ent.ConsentStamp, error) {
	err := s.ConsentStamp().Create().
		SetChannel(k.Channel).
		SetDestination(k.Destination).
		SetSendingSource(k.Source).
		OnConflictColumns(consentstamp.FieldWorkspaceID, consentstamp.FieldChannel, consentstamp.FieldDestination, consentstamp.FieldSendingSource).
		DoNothing().
		Exec(ctx)
	if _, err := db.Inserted(err); err != nil {
		return nil, fmt.Errorf("make the consent stamp: %w", err)
	}
	q := s.ConsentStamp().Query().Where(of(k)...)
	q.Modify(func(sel *entsql.Selector) { sel.For(entsql.LockNoKeyUpdate) })
	row, err := q.Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock the consent stamp: %w", err)
	}
	return row, nil
}

// Admit reports whether an observation that occurred at o may set fact of k, and stamps it when it
// may: when nothing has set the state yet, or o occurred later than what did. Within one bot the
// later is the higher number; otherwise the later time, and of two of the same moment the refusal
// wins (refusal: o stops or withdraws consent), so a tie leaves an opt-out standing. A state
// already as o would set it is stamped too: the person said it again, later. An observation not
// admitted changes nothing. On sms the stamp an Erasure kept by the number's hash counts as well.
func Admit(ctx context.Context, s *ent.Scoped, k Key, fact Fact, o Occurrence, refusal bool) (bool, error) {
	if o.At.IsZero() {
		return false, ErrNoOccurrence
	}
	// The database keeps microseconds: an observation is compared as it would be read back.
	o.At = o.At.UTC().Truncate(time.Microsecond)
	row, err := Hold(ctx, s, k)
	if err != nil {
		return false, err
	}
	standing := []*ent.ConsentStamp{row}
	if k.Channel == channels.SMS {
		hashed, err := s.ConsentStamp().Query().Where(of(Key{Channel: k.Channel, Destination: phone.Hash(k.Destination), Source: k.Source})...).Only(ctx)
		switch {
		case err == nil:
			standing = append(standing, hashed)
		case !ent.IsNotFound(err):
			return false, fmt.Errorf("read the erased consent stamp: %w", err)
		}
	}
	for _, st := range standing {
		if !later(o, stampOf(st, fact), refusal) {
			return false, nil
		}
	}
	update := s.ConsentStamp().UpdateOneID(row.ID)
	switch fact {
	case OptOut:
		update.SetOptOutOccurredAt(o.At)
		if o.IntegrationID != 0 && o.Sequence != 0 {
			update.SetOptOutIntegrationID(o.IntegrationID).SetOptOutSequence(o.Sequence)
		} else {
			update.ClearOptOutIntegrationID().ClearOptOutSequence()
		}
	case Confirmation:
		update.SetConfirmationOccurredAt(o.At)
	}
	if err := update.Exec(ctx); err != nil {
		return false, fmt.Errorf("stamp the consent: %w", err)
	}
	return true, nil
}

// stampOf is the Occurrence that set fact on row, nil when nothing has.
func stampOf(row *ent.ConsentStamp, fact Fact) *Occurrence {
	switch fact {
	case OptOut:
		if row.OptOutOccurredAt == nil {
			return nil
		}
		o := Occurrence{At: *row.OptOutOccurredAt}
		if row.OptOutIntegrationID != nil && row.OptOutSequence != nil {
			o.IntegrationID, o.Sequence = *row.OptOutIntegrationID, *row.OptOutSequence
		}
		return &o
	case Confirmation:
		if row.ConfirmationOccurredAt == nil {
			return nil
		}
		return &Occurrence{At: *row.ConfirmationOccurredAt}
	}
	return nil
}

// later reports whether o may replace the state standing set: see Admit.
func later(o Occurrence, standing *Occurrence, refusal bool) bool {
	switch {
	case standing == nil:
		return true
	case o.IntegrationID != 0 && o.Sequence != 0 && o.IntegrationID == standing.IntegrationID && standing.Sequence != 0:
		return o.Sequence > standing.Sequence
	case o.At.After(standing.At):
		return true
	case o.At.Equal(standing.At):
		return refusal
	default:
		return false
	}
}

func of(k Key) []predicate.ConsentStamp {
	return []predicate.ConsentStamp{
		consentstamp.ChannelEQ(k.Channel),
		consentstamp.DestinationEQ(k.Destination),
		consentstamp.SendingSourceEQ(k.Source),
	}
}
