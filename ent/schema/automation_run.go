package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
)

// AutomationRun is one contact's progress through an Automation, an Enrollment. A Contact may have
// several in an Automation, one after another (ADR 0127): at most one active per key value, the
// partial unique index over active Enrollments, and each numbered by its ordinal. Whether a Contact
// enrolls again is the Entry policy of the version, judged under the Contact's lock. One Event, or
// one Rule Firing, enrolls a Contact in an Automation at most once (ADR 0127): the unique indexes on
// the triggering Event's source id and on the Firing. chain names the Automations the chain that
// made it passed through, its own included. current_step_id names the Step it stands on; resume_at
// is when the wait it stands on is due.
type AutomationRun struct {
	ent.Schema
}

// Annotations adds to the enum checks that an Enrollment began on an Event's Trigger, on a Rule's
// Firing or on an occurrence of a date Trigger, never two, that its chain is a list and that its ordinal counts from one.
func (AutomationRun) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("automation_runs", AutomationRun{}.Fields())
	a.Checks["automation_runs_entry_check"] = "num_nonnulls(trigger_source_id, firing_id, occurrence) <= 1"
	a.Checks["automation_runs_chain_check"] = jsonCheck("chain", "array")
	a.Checks["automation_runs_ordinal_check"] = "ordinal >= 1"
	a.Checks["automation_runs_deferral_check"] = "(deferred_since IS NULL) = (deferral_cause IS NULL)"
	return []schema.Annotation{a}
}

func (AutomationRun) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "automation_runs"}}
}

func (AutomationRun) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("automation_id"),
		// The Enrollment dies with its Contact (ON DELETE CASCADE; Erasure deletes it).
		field.Int64("contact_id"),
		// The Automation version the Enrollment began on (ADR 0049): current_step_id names one
		// of its Steps, and the worker reads them from it, so an edit never moves the Step an
		// Enrollment stands on.
		field.Int64("version_id"),
		// exited: the enrollment left early (e.g. an unsubscribe, a suppression or an exit
		// event mid-run) — distinct from completing the sequence. goal_reached: the Contact
		// did what the Automation was for (its goal event) before the steps ran out.
		field.Enum("status").
			Annotations(Machine()).
			Values("active", "completed", "failed", "exited", "goal_reached").
			Default("active"),
		// When the Enrollment began (#751): the occurred_at of the Event that triggered it, or of
		// the Event a Rule fired on when a Rule's Action enrolled. An exit or goal Event ends it
		// only when it occurred at this moment or later, so loaded or late history does not.
		field.Time("began_at").
			Annotations(Attribute(reasonWithStatus)).
			Default(time.Now).
			Immutable().
			Annotations(entsql.Default("CURRENT_TIMESTAMP")),
		// The triggering Event the steps bind as `event` (#752): its action and its properties,
		// copied at Enroll from the Event that began the Enrollment (the Event a Rule fired on,
		// when a Rule's Action enrolled) and never changed. A snapshot, not a reference to the
		// Event: Retention (ADR 0019) may remove the Event during a long wait, and the copy stays.
		// The properties are bounded as a Rule sees them (events.BoundedProperties): none when the
		// Event weighed more. Both are empty for an Enrollment no Event began. Personal data: they
		// go with the row on Erasure (ADR 0021).
		field.String("event_action").
			Optional().
			Nillable().
			Immutable(),
		field.JSON("event_properties", map[string]any{}).
			Optional().
			Immutable(),
		// The source id of the Event whose Trigger began the Enrollment (events.Envelope.SourceID,
		// ADR 0127): one Event enrolls a Contact in an Automation once. A snapshot, without a key:
		// Retention (ADR 0019) and an Erasure remove the Event, and the Enrollment keeps its id.
		// Null for an Enrollment a Rule made.
		field.String("trigger_source_id").
			Optional().
			Nillable().
			Immutable(),
		// The Rule Firing whose Enroll Action made the Enrollment (ADR 0127): one Firing enrolls a
		// Contact in an Automation once. Cleared when retention prunes the Firing (SET NULL), and
		// null for an Enrollment a Trigger began.
		field.Int64("firing_id").
			Optional().
			Nillable().
			Immutable(),
		// The occurrence of a date Trigger the Enrollment was made for (ADR 0126): the anniversary
		// year of a yearly date or a once date's one (datetrigger.Occurrence, an int column). It belongs to the Automation,
		// not the version: one Enrollment per Contact and occurrence, whatever field a later version
		// names. Null for an Enrollment an Event or a Rule began.
		field.Int("occurrence").
			GoType(datetrigger.StoredOccurrence(0)).
			Optional().
			Nillable().
			Immutable(),
		// The ids of the Automations the chain that made the Enrollment passed through, its own
		// included, ascending (ADR 0127): the list of the Event or the Firing's Event that caused
		// it, and its Automation. Everything published on behalf of the Enrollment carries it
		// (events.Envelope.Automations), and an entry whose cause names the Automation is skipped
		// as `cycle`; a Step resumed after a wait reads it here. A snapshot, without keys: an
		// Automation it names may be archived or gone, and the list still says where the chain
		// went.
		field.JSON("chain", []int64{}).
			Default([]int64{}).
			Immutable().
			Annotations(entsql.Default("[]")),
		// Which of the Contact's Enrollments in the Automation it is (ADR 0127): 1 for the first,
		// plus one for each after it, counted under the Contact's lock when it is made. An
		// Enrollment made already ended counts like any other. A Split's draw reads it, so a
		// Contact is not held on one path for good.
		field.Int("ordinal").
			Immutable(),
		// The value of the version's Enrollment key the triggering Event carried (ADR 0165), the
		// property's string form as Postgres reads it (properties ->> key): one active Enrollment
		// per value, the partial unique index. Null for a version with no key, so a Contact has one
		// active Enrollment in the Automation (NULLS NOT DISTINCT). An exit or goal Event carrying
		// the property ends only the Enrollment of its value. Personal data: it goes with the row
		// on Erasure (ADR 0021).
		field.String("key_value").
			Optional().
			Nillable().
			Immutable(),
		// When the Enrollment left `active` (#753): written by the one move out of it
		// (internal/automations, runstate.go), together with the terminal status, and never again.
		// An exit or goal Event ends it when that Event occurred, on began_at's clock; a step or an
		// unsubscribe ends it at its caller's clock. Null while it is active. The Automation report
		// counts an outcome on the Report day of this moment and measures a duration from began_at.
		field.Time("ended_at").
			Annotations(Attribute(reasonWithStatus)).
			Optional().
			Nillable(),
		// The id of the Step of its version the Enrollment stands on, the Step its next job
		// executes (ADR 0122): an id every version keeps, so a Step inserted above it in a later
		// version moves nothing. A wait holds the Enrollment on itself until it is due (resume_at,
		// ADR 0124), and the move that leaves the last Step completes the Enrollment there, so a
		// terminal Enrollment keeps the Step it ended on. Null only for a version of no Steps.
		field.String("current_step_id").
			GoType(automationstep.ID("")).
			Optional().
			Nillable(),
		// When the wait the Enrollment stands on is due: a wait Step's due moment, or, while its send
		// Step is deferred, when the send is tried again (ADR 0178).
		field.Time("resume_at").
			Annotations(Attribute("the wake-up time of a waiting run, written with its status by its machine")).
			Optional().
			Nillable(),
		// Since when and why the send of the Step it stands on waits (ADR 0178): the moment of the
		// first Deferral at the Step, kept by every later one, and the cause of the latest. Both
		// are cleared by the write that moves the Enrollment off the Step, and are null while its
		// send is not deferred (automation_runs_deferral_check).
		field.Time("deferred_since").
			Annotations(Attribute("records since when the send of the current Step waits; set by its Deferral, cleared by the move off the Step")).
			Optional().
			Nillable(),
		field.Enum("deferral_cause").
			GoType(deferralcause.Cause("")).
			Annotations(Attribute("records why the send of the current Step waits; set by its Deferral, cleared by the move off the Step")).
			Optional().
			Nillable(),
		// Why a failed enrollment failed, set by an HTTP step (ADR 0039): the closed error
		// kind, the receiver's status for http_status, and the first 1 KiB of its answer.
		// Erased with the row (ADR 0021).
		field.Enum("failure_kind").
			Values("invalid_body", "timeout", "blocked_address", "http_status", "network").
			Optional().
			Nillable(),
		field.Int("failure_status").
			Optional().
			Nillable(),
		field.String("failure_excerpt").
			Optional().
			Nillable(),
	}
}

func (AutomationRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("automation", Automation.Type).
			Ref("runs").
			Field("automation_id").
			Required().
			Unique(),
		edge.From("version", AutomationVersion.Type).
			Ref("runs").
			Field("version_id").
			Required().
			Unique(),
		edge.To("contact", Contact.Type).Field("contact_id").Required().Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("firing", RuleFiring.Type).Field("firing_id").Unique().Immutable().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		// What the Enrollment kept of each Step it reached (ADR 0124); their key is theirs.
		edge.From("step_records", AutomationStepRecord.Type).Ref("run"),
	}
}

func (AutomationRun) Indexes() []ent.Index {
	return []ent.Index{
		// One active Enrollment per Automation, Contact and key value (ADR 0127, ADR 0165), a null
		// key value one value (NullsNotDistinct): the entry checks find none under the Contact's
		// lock before they make one.
		index.Fields("workspace_id", "automation_id", "contact_id", "key_value").
			Unique().
			Annotations(entsql.IndexWhere("status = 'active'")).
			StorageKey(ActiveEnrollmentIndex),
		// The Contact's Enrollments in the Automation, numbered: the entry checks read them, and
		// one ordinal is one Enrollment.
		index.Fields("workspace_id", "automation_id", "contact_id", "ordinal").
			Unique().
			StorageKey("automation_runs_workspace_automation_contact_ordinal"),
		// One Enrollment per Automation and triggering Event, and per Automation and Rule Firing
		// (ADR 0127): a redelivered Event or a Firing run again makes nothing.
		index.Fields("workspace_id", "automation_id", "trigger_source_id").
			Unique().
			StorageKey("automation_runs_workspace_automation_trigger_source"),
		index.Fields("workspace_id", "automation_id", "firing_id").
			Unique().
			StorageKey("automation_runs_workspace_automation_firing"),
		// One Enrollment per Automation, Contact and occurrence of its date Trigger (ADR 0126).
		index.Fields("workspace_id", "automation_id", "contact_id", "occurrence").
			Unique().
			Annotations(entsql.IndexWhere("occurrence IS NOT NULL")).
			StorageKey(OccurrenceEnrollmentIndex),
		// The firing foreign key's index.
		index.Fields("workspace_id", "firing_id"),
		// Erasure reaches runs by contact; the leading workspace_id also serves workspace scans.
		index.Fields("workspace_id", "contact_id"),
		// The contact foreign key's index: deleting a Contact finds its runs by it.
		index.Fields("contact_id").
			StorageKey("automation_runs_contact_id"),
		// "Does an Enrollment stand on this version" decides rewrite or supersede.
		index.Fields("workspace_id", "version_id"),
		// Back an Automation's Results: its Enrollments that began, and those that ended, in a
		// window, so the report reads the window and not every Enrollment the Automation ever had.
		// The leading workspace_id and automation_id also serve the Automation's composite foreign
		// key.
		index.Fields("workspace_id", "automation_id", "began_at"),
		index.Fields("workspace_id", "automation_id", "ended_at"),
	}
}

// OccurrenceEnrollmentIndex is the partial unique index of a Contact's Enrollment in an Automation
// per occurrence of its date Trigger.
const OccurrenceEnrollmentIndex = "automation_runs_occurrence"

// ActiveEnrollmentIndex is the partial unique index of a Contact's active Enrollment in an
// Automation per key value.
const ActiveEnrollmentIndex = "automation_runs_active_enrollment"
