package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
)

// AutomationSkippedEntry is an entry into an Automation that its entry checks refused (ADR 0127):
// the Contact, the triggering Event's source id or the Rule Firing whose Enroll Action tried, why
// it was refused and when the entry would have begun. It is unique per Automation and Event (or
// Firing), so a redelivery records nothing new. It is analytical history: retention prunes it with
// the analytical Events (ADR 0019), and it goes with its Contact on Erasure.
type AutomationSkippedEntry struct {
	ent.Schema
}

// Annotations adds to the enum checks that an entry came from an Event's Trigger, from a Rule's
// Firing or from an occurrence of a date Trigger, never two.
func (AutomationSkippedEntry) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("automation_skipped_entries", AutomationSkippedEntry{}.Fields())
	a.Checks["automation_skipped_entries_entry_check"] = "num_nonnulls(source_id, firing_id, occurrence) <= 1"
	return []schema.Annotation{a}
}

func (AutomationSkippedEntry) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "automation_skipped_entries"}}
}

func (AutomationSkippedEntry) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("automation_id").
			Immutable(),
		// The skipped entry dies with its Contact (ON DELETE CASCADE; Erasure deletes it).
		field.Int64("contact_id").
			Immutable(),
		// The source id of the triggering Event (events.Envelope.SourceID). A snapshot, without a
		// key: Retention (ADR 0019) and an Erasure remove the Event. Null for a Rule's entry.
		field.String("source_id").
			Optional().
			Nillable().
			Immutable(),
		// The Rule Firing whose Enroll Action was refused; null for a Trigger's entry. The skipped
		// entry goes with it (CASCADE): both are kept for the analytical window.
		field.Int64("firing_id").
			Optional().
			Nillable().
			Immutable(),
		// The occurrence of a date Trigger the entry was for (ADR 0126): the anniversary year of a
		// yearly date or a once date's one (datetrigger.Occurrence, an int column). Null for an Event's or a
		// Rule's entry.
		field.Int("occurrence").
			GoType(datetrigger.StoredOccurrence(0)).
			Optional().
			Nillable().
			Immutable(),
		// Why the entry was refused, by the first entry check that refused it, in their order:
		// `no_enrollment_key` (the version has an Enrollment key and the triggering Event lacks the
		// property, ADR 0165), `cycle` (the chain that caused it passed through the Automation
		// already), `active_enrollment` (an Enrollment of the Contact for the key value was active
		// at the Event's moment, or is active still), `once` (the Contact had an Enrollment for the
		// key value in the Automation before, under the `once` Entry policy), `cooldown` (under
		// `again`, the cooldown from another Enrollment's beginning for the key value had not
		// passed) or `enrollment_ceiling` (a keyed version's Contact had 10 active Enrollments, or
		// 10 begun within a day of the entry, ADR 0165). An occurrence of a date Trigger (ADR 0126) runs
		// only `active_enrollment`, and is `missed` when the scheduler reached it 24 hours late or
		// more while its date was written by its moment, or `send_locked` while its date was written
		// through a Connection without mcp:send (ADR 0132): such an occurrence is not decided, and a
		// later write may still enroll it.
		field.Enum("reason").
			Values("no_enrollment_key", "cycle", "active_enrollment", "once", "cooldown", "enrollment_ceiling", "missed", "send_locked").
			Immutable(),
		// When the entry would have begun, as an Enrollment's began_at: the occurred_at of its Event
		// (of the Event the Rule fired on). The report counts it on this moment's Report day.
		field.Time("occurred_at").
			Annotations(Attribute(reasonHistory)).
			Immutable(),
	}
}

func (AutomationSkippedEntry) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("automation", Automation.Type).Field("automation_id").Unique().Required().Immutable().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("contact", Contact.Type).Field("contact_id").Unique().Required().Immutable().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("firing", RuleFiring.Type).Field("firing_id").Unique().Immutable().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (AutomationSkippedEntry) Indexes() []ent.Index {
	return []ent.Index{
		// One skipped entry per Automation and Event, and per Automation and Firing.
		index.Fields("workspace_id", "automation_id", "source_id").
			Unique().
			StorageKey("automation_skipped_entries_workspace_automation_source"),
		index.Fields("workspace_id", "automation_id", "firing_id").
			Unique().
			StorageKey("automation_skipped_entries_workspace_automation_firing"),
		// One decided skip per Automation, Contact and occurrence (ADR 0126), and one record of an
		// occurrence held by the send lock, which is not decided.
		index.Fields("workspace_id", "automation_id", "contact_id", "occurrence").
			Unique().
			Annotations(entsql.IndexWhere(OccurrenceSkipWhere)).
			StorageKey(OccurrenceSkipIndex),
		index.Fields("workspace_id", "automation_id", "contact_id", "occurrence").
			Unique().
			Annotations(entsql.IndexWhere(OccurrenceSendLockedWhere)).
			StorageKey(OccurrenceSendLockedIndex),
		// The report counts an Automation's skipped entries in a window; the list reads them newest
		// first.
		index.Fields("workspace_id", "automation_id", "occurred_at"),
		// The contact foreign key's index: deleting a Contact finds its skipped entries by it.
		index.Fields("workspace_id", "contact_id"),
		// The firing foreign key's index.
		index.Fields("workspace_id", "firing_id"),
		// The retention prune (ADR 0019) selects the skipped entries older than the window.
		index.Fields("created_at"),
	}
}

// The partial unique indexes of an occurrence's skipped entries (ADR 0126): the one that decides it
// (`active_enrollment` or `missed`), and the one record of it held by the send lock.
const (
	OccurrenceSkipIndex       = "automation_skipped_entries_occurrence"
	OccurrenceSendLockedIndex = "automation_skipped_entries_occurrence_send_locked"
)

// The predicates of the two partial indexes, which an insert on one names in its ON CONFLICT
// clause: Postgres infers a partial index only from a predicate that implies the index's.
const (
	OccurrenceSkipWhere       = "occurrence IS NOT NULL AND reason <> 'send_locked'"
	OccurrenceSendLockedWhere = "occurrence IS NOT NULL AND reason = 'send_locked'"
)
