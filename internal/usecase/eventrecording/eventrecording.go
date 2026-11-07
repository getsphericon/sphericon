// Package eventrecording is the use case that records a customer's Events in the Event log
// (GLOSSARY, ADR 0102): Record resolves each Event's identity to an existing Contact (contacts,
// ADR 0002), checks an order Event's shape (orders) and publishes the batch on the bus in a single
// transaction. It writes no table itself: the bus stores the Events, and the Event log's reads are
// the bus's (events.ListLog, events.DistinctActions).
//
// Publishing an Event writes its row in Record's transaction; what reacts to it
// (Automations, Rules, webhooks) runs asynchronously, so Record is accept-then-process.
package eventrecording

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/batch"
)

// UseCase records a customer's Events.
type UseCase struct {
	bus  *events.Bus
	live LivePolicy
}

// LivePolicy says how old a live Event may be: an older one is history and arrives as a
// Backfill, never as a live Event that could enroll an Automation. Now is the clock the
// age is judged against.
type LivePolicy struct {
	Window time.Duration
	Now    func() time.Time
}

// New builds the use case over the domain-event bus (transactional publish). Identity
// resolution goes through the Workspace-scoped client each call receives.
func New(bus *events.Bus, live LivePolicy) *UseCase {
	return &UseCase{bus: bus, live: live}
}

// Input is one customer Event to record. Action and Properties are the customer's
// own and are stored as-is; the alias keys (SubjectID, Email, Phone) identify the
// Contact it belongs to, if one exists.
type Input struct {
	SubjectID  string
	Email      *string
	Phone      *string
	Action     string
	OccurredAt time.Time // zero => publish time
	Properties map[string]any
	// ClientEventID is the client's own id for the Event ("" = none). A repeat of an id in
	// a Workspace is a no-op, so a client can resend a load it never saw answered.
	ClientEventID string
	// Backfill records the Event as history: no Automation enrolls on it and no webhook
	// endpoint receives it.
	Backfill bool
}

// validate refuses an Event the use case must not publish: one without an identity or an
// action, one that claims a reserved action, a Backfill without a time that has passed,
// and a live Event old enough to be history.
func (m *UseCase) validate(in Input) error {
	if strings.TrimSpace(in.SubjectID) == "" {
		return events.Reject("subject id and action are required")
	}
	if err := events.CheckAction(in.Action); err != nil {
		return err
	}
	now := m.live.Now()
	switch {
	case in.Backfill && in.OccurredAt.IsZero():
		return events.Reject("a backfilled event needs occurredAt")
	case in.Backfill && in.OccurredAt.After(now):
		return events.Reject("occurredAt of a backfilled event must not be in the future")
	case !in.Backfill && !in.OccurredAt.IsZero() && in.OccurredAt.Before(now.Add(-m.live.Window)):
		return events.Reject(fmt.Sprintf("occurredAt is more than %d hours ago: send history with backfill", int(m.live.Window.Hours())))
	}
	return nil
}

// Record attaches each Event to an existing Contact by stable identity (an unknown
// identity stays anonymous; a Contact is never created) and publishes the whole batch
// atomically: either every Event is accepted or none is. An Event that fails validate
// refuses the whole batch with an events.ErrRejected.
func (m *UseCase) Record(ctx context.Context, s *ent.Scoped, inputs []Input) error {
	for _, in := range inputs {
		if err := m.validate(in); err != nil {
			return err
		}
	}
	check := orders.NewCheck(s)
	for _, in := range inputs {
		if err := check.Check(ctx, in.Action, in.Properties); err != nil {
			return err
		}
	}
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		for _, in := range inputs {
			// A client event id is the Event's source id. A repeat of it, inside this batch or
			// from an earlier request, is the same Event: Publish finds its row and publishes
			// nothing, so it is recorded and reacted to once.
			dedupID := ""
			if in.ClientEventID != "" {
				dedupID = events.DedupKey(events.SourceAPI, ts.WorkspaceID(), in.ClientEventID)
			}
			contactID, err := contacts.ResolveID(ctx, ts, in.SubjectID, in.Email, in.Phone)
			if err != nil {
				return err
			}
			collected := &events.CollectedEvent{
				WorkspaceID: ts.WorkspaceID(),
				ContactID:   contactID,
				SubjectID:   in.SubjectID,
				Action:      in.Action,
				OccurredAt:  in.OccurredAt,
				Properties:  in.Properties,
				DedupID:     dedupID,
				Backfill:    in.Backfill,
			}
			if in.Email != nil {
				collected.Email = *in.Email
			}
			if in.Phone != nil {
				collected.Phone = *in.Phone
			}
			if err := pub.Publish(ctx, collected); err != nil {
				return err
			}
		}
		return nil
	})
}

// RecordEach records each Event in its own transaction, so an invalid or failing item
// does not affect the others (Record, by contrast, is all-or-nothing). The outcomes are in the
// order of inputs; an outcome without an error is an accepted Event.
func (m *UseCase) RecordEach(ctx context.Context, s *ent.Scoped, inputs []Input) batch.Result[struct{}] {
	return batch.Each(inputs, func(in Input) (struct{}, error) {
		return struct{}{}, m.Record(ctx, s, []Input{in})
	})
}
