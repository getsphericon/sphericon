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

// Module orders the Consent observations. It holds nothing: every call takes the scope whose
// transaction it runs in.
type Module struct{}

// New builds the module.
func New() *Module { return &Module{} }

// Hold locks the stamp row of k to the end of the transaction, making it if there is none, and
// returns it: the observations of one key are handled one at a time, so a comparison and the write
// it allows are one step. An owner holds it before it reads the state it decides on, and before
// the Contact's lock (contactlock), which its write may take.
func (m *Module) Hold(ctx context.Context, s *ent.Scoped, k Key) (*ent.ConsentStamp, error) {
	k = Stored(k)
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

// Stored is the key k's stamp is stored by: on sms the number's hash (phone.Hash), never the
// number, so a stamp holds nothing an Erasure would have to find and remove (ADR 0021); on the
// other Channels the Destination, as its opt-out is kept.
func Stored(k Key) Key {
	if k.Channel == channels.SMS {
		k.Destination = phone.Hash(k.Destination)
	}
	return k
}

// Observation is a Consent observation of a key: the state it sets, when it occurred, and whether
// it stops or withdraws consent (a refusal) rather than giving it.
type Observation struct {
	Key      Key
	Fact     Fact
	Occurred Occurrence
	// Refusal is true for an observation that stops or withdraws consent.
	Refusal bool
}

// Admit reports whether the observation in may set its Fact of its Key, and stamps it when it may:
// when nothing has set the state yet, or it occurred later than what did. Within one bot the later
// is the higher number; otherwise the later time, and of two of the same moment the refusal wins,
// so a tie leaves an opt-out standing. A state already as the observation would set it is stamped
// too: the person said it again, later. An observation not admitted changes nothing. The stamp is
// Hold's, stored by Stored's key.
func (m *Module) Admit(ctx context.Context, s *ent.Scoped, in Observation) (bool, error) {
	k, fact, o, refusal := in.Key, in.Fact, in.Occurred, in.Refusal
	if o.At.IsZero() {
		return false, ErrNoOccurrence
	}
	// The database keeps microseconds: an observation is compared as it would be read back.
	o.At = o.At.UTC().Truncate(time.Microsecond)
	row, err := m.Hold(ctx, s, k)
	if err != nil {
		return false, err
	}
	if !later(o, stampOf(row, fact), refusal) {
		return false, nil
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

// later reports whether o may replace the state standing set: see Module.Admit.
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
