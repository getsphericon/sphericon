package events

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// Cause is what brought an event in, when it was not a live happening (ADR 0138 as amended): an
// import job, or a Backfill. It travels on the envelope (Envelope.Cause), stamped from the context
// it was published in (WithCause), and it is not CausedBy, which names a Rule's Action. An event
// that is History is recorded but is never a trigger: it enrolls no one, fires no Rule, ends no
// Enrollment as an exit or goal event, satisfies no Wait for event and reaches no Webhook endpoint.
// What it holds back is the event as a trigger, nothing else: segmentation, Orders and a refusal's
// own effects read it as any other.
type Cause struct {
	// Import is the id of the import job that brought the event, 0 for any other.
	Import int64 `json:"import,omitempty"`
	// History marks the event as history (IsHistory). An import's events are, unless the Member chose
	// "treat them as new sign-ups": those still name the job but are live.
	History bool `json:"history,omitempty"`
}

// Backfill is the cause of an event loaded as history (ADR 0044), and of a Confirmation recorded as
// history (a grandfathered one, ADR 0013): no import names it.
func Backfill() Cause { return Cause{History: true} }

// Import is the cause of an event the import job jobID brought: history, as a Backfill's.
func Import(jobID int64) Cause { return Cause{Import: jobID, History: true} }

// ImportAsNew is the cause of an event the import job jobID brought when the Member chose to treat
// its Contacts as new sign-ups: it names the job, but it is live.
func ImportAsNew(jobID int64) Cause { return Cause{Import: jobID} }

type causeOfKey struct{}

// WithCause returns ctx for publishing what c brought: every event published in it carries c on
// its envelope. Like SendLocked, it is not carried into the jobs the write queues.
func WithCause(ctx context.Context, c Cause) context.Context {
	return context.WithValue(ctx, causeOfKey{}, c)
}

// causeIn is the Cause ctx carries, none outside WithCause.
func causeIn(ctx context.Context) Cause {
	c, _ := ctx.Value(causeOfKey{}).(Cause)
	return c
}

// IsHistory is the one predicate of whether an event is history, never a trigger: every consumer
// that reacts to an event as a trigger reads it, here or through the stored Event's forms below
// (Live, WasHistory), which read the flag the envelope's cause stored (events.backfill). A static
// check (ast-grep, stored-history-through-the-predicate) refuses reading that flag anywhere else.
func IsHistory(env Envelope) bool { return env.Cause.History }

// Live narrows a query of stored Events to the ones that are not history (IsHistory).
func Live() predicate.Event { return event.Backfill(false) }

// WasHistory reports whether the stored Event e is history (IsHistory).
func WasHistory(e *ent.Event) bool { return e.Backfill }
