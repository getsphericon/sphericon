package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/skipreason"

	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// AutomationStepRecord is what an Enrollment keeps of one Step it reached (ADR 0124): when it
// reached and left it, how it left it (at a fork, the path it took and, at a Branch, each path's
// outcome) and, at a wait, the moment the wait is due or, at a Wait until that has none, why the
// Enrollment went on at once. One per
// Enrollment and Step: a version's Steps form a tree with no cycles. It is written in the
// transaction of the move that reaches or leaves the Step, which the Enrollment's own
// compare-and-set on its cursor decides, and goes with its Enrollment (ON DELETE CASCADE, so an
// Erasure of the Contact takes it). A send's own result stays on its Outbound message.
type AutomationStepRecord struct {
	ent.Schema
}

// Annotations adds to the enum checks that only a left Step record has an outcome (one the
// Enrollment ended on through an exit, a goal, a failure or an unsubscribe is left with none),
// that only a skipped Step says why, and that a Wait until either waits or went on at once.
func (AutomationStepRecord) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("automation_step_records", AutomationStepRecord{}.Fields())
	a.Checks["automation_step_records_left_check"] = "outcome IS NULL OR left_at IS NOT NULL"
	a.Checks["automation_step_records_reason_check"] = "outcome IS NOT NULL OR skip_reason IS NULL"
	a.Checks["automation_step_records_skipped_check"] = variantCheck("outcome", stepOutcome)
	a.Checks["automation_step_records_wait_check"] = "due_at IS NULL OR went_on IS NULL"
	a.Checks["automation_step_records_awaited_check"] = "(awaited_event_id IS NULL) = (awaited_action IS NULL)"
	a.Checks["automation_step_records_http_body_check"] = "http_body IS NULL OR (http_status IS NOT NULL AND http_dropped IS NULL)"
	return []schema.Annotation{a}
}

// stepOutcome is the column that explains each outcome: a skipped Step its reason, a fork the
// path it sent the Enrollment down and, at a Branch, each path's outcome and, at a Wait for event
// that arrived on an Event, the copy of that Event; an HTTP step that is done, the response it kept
// and why it dropped the body.
var stepOutcome = []variant{
	{value: "done", optional: []string{"http_status", "http_body", "http_dropped"}},
	{value: "skipped", required: []string{"skip_reason"}},
	{value: "path", required: []string{"path_id"}, optional: []string{"path_outcomes", "awaited_event_id", "awaited_action", "awaited_properties"}},
}

func (AutomationStepRecord) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "automation_step_records"}}
}

func (AutomationStepRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The Enrollment that reached the Step.
		field.Int64("run_id").
			Immutable(),
		// The id of the Step in the Enrollment's version (ADR 0122).
		field.String("step_id").
			GoType(automationstep.ID("")).
			Immutable(),
		field.Time("reached_at").
			Annotations(Attribute("records when the Enrollment reached the Step, in the move its cursor's compare-and-set decides")).
			Immutable(),
		// Null while the Enrollment stands on the Step. Set once, when it leaves the Step: by the
		// move that takes it on, or by the end of the Enrollment on it.
		field.Time("left_at").
			Annotations(Latch()).
			Optional().
			Nillable(),
		// done: the Step did its work (a send's own result is its Outbound message's; a Wait until
		// that went on at once is done too, went_on saying why). skipped:
		// the Enrollment moved on without it, for skip_reason. path: a fork sent the Enrollment
		// down the path of path_id (ADR 0124). Null on a Step the Enrollment ended
		// on through an exit, a goal, a failure or an unsubscribe: its end says why. Written in
		// the latch of left_at, never after it.
		field.Enum("outcome").
			Values("done", "skipped", "path").
			Annotations(Latch()).
			Optional().
			Nillable(),
		// Why a skipped Step was skipped; the reasons are skipreason's, the one closed list (ADR
		// 0178).
		field.Enum("skip_reason").
			GoType(skipreason.Reason("")).
			Optional().
			Nillable(),
		// The path of its fork the Enrollment was sent down, by the path's id (ADR 0124): decided
		// once, and written in the latch of left_at with the outcome `path`, never after it. Null
		// for a Step that is no fork.
		field.String("path_id").
			GoType(automationstep.ID("")).
			Optional().
			Nillable(),
		// At a Branch, what each path's Condition was for the Contact when the Enrollment reached
		// it, by path id: true, false or unknown (ADR 0123), never the values it tested. Written
		// with path_id; null for any other Step.
		field.JSON("path_outcomes", map[automationstep.ID]conditiontree.Truth{}).
			Optional(),
		// When a wait is due: computed once, when the Enrollment reached it, so a restart neither
		// moves it early nor makes it wait twice. Null for any other Step.
		field.Time("due_at").
			Annotations(Attribute("records the due moment a wait computed once when reached; the Enrollment's resume_at is what its job reads")).
			Optional().
			Nillable().
			Immutable(),
		// Why the Enrollment went on at once from a Wait until with no moment ahead within 366 days
		// (ADR 0126): past, no_date or beyond_bound. Decided once, when it reached the Step, as a
		// wait's due_at is, and never both. Null for any other Step, and for a Wait until that waited.
		field.Enum("went_on").
			GoType(automationstep.WentOn("")).
			Optional().
			Nillable().
			Immutable(),
		// At a Wait for event on an open or a click, the path it decided when the Enrollment
		// reached it, in the transaction of that move (ADR 0124): `unknown` when the message went out
		// untracked, was never sent or had its tracking withdrawn, `arrived` when it was already
		// opened or clicked. Its job, queued at once, takes it, whatever happened since; its record
		// keeps its deadline all the same. Null for one that waits, and for any other Step.
		field.String("decided_path_id").
			GoType(automationstep.ID("")).
			Optional().
			Nillable().
			Immutable(),
		// At a Wait for event that took `arrived` on an Event, the copy of that Event the Steps
		// after it read under its name (ADR 0136): its action and its properties, bounded as the
		// Enrollment's own `event` is (events.BoundedProperties, none when the Event weighed more),
		// and the id of the Event it was copied from. Written once, in the latch of left_at with path_id; cleared, all three, in the
		// move that ends the Enrollment, whatever its end, since nothing reads it after and it may
		// hold the person's own words. Null for any other Step.
		field.Int64("awaited_event_id").
			Optional().
			Nillable().
			Annotations(ScopedRef{
				Unchecked: true,
				Reason: "a snapshot of the Event the copy was taken from, as the Enrollment's own triggering " +
					"Event's source id is: Retention may delete the Event during the wait while the copy stays",
			}),
		field.String("awaited_action").
			Optional().
			Nillable(),
		field.JSON("awaited_properties", map[string]any{}).
			Optional(),
		// At a Wait for event on an Event with a Condition, the Events its wake handled while the
		// Enrollment waited and found the Condition false for, by id (ADR 0124): the Condition is
		// read when an Event is handled, so the timeout never takes `arrived` on one of them,
		// whatever the Contact's data became since. Null while there is none, and for any other Step.
		// A list with no key: a record of what the wake decided, read only while the Enrollment
		// waits, and an Event deleted since is no candidate of the timeout anyway.
		field.JSON("declined_event_ids", []int64{}).
			Optional(),
		// At an HTTP step that is done, the response the Steps after it read under its name (ADR
		// 0161): its status code, a 2xx, and its body when it was a JSON object within the bounds of
		// an Event's properties (events.BoundedProperties), else null with http_dropped saying why.
		// Written once, in the latch of left_at with the outcome `done`. The status and the body are
		// cleared in the move that ends the Enrollment, whatever its end, as an awaited Event's copy
		// is, since nothing reads them after and the body is a stranger's data; http_dropped stays,
		// as the record of what the Step did. Null for any other Step.
		field.Int32("http_status").
			Optional().
			Nillable(),
		field.JSON("http_body", map[string]any{}).
			Optional(),
		field.Enum("http_dropped").
			GoType(automationstep.BodyDropped("")).
			Optional().
			Nillable(),
	}
}

func (AutomationStepRecord) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("run", AutomationRun.Type).Field("run_id").Required().Unique().Immutable().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (AutomationStepRecord) Indexes() []ent.Index {
	return []ent.Index{
		// One record per Enrollment and Step. It also serves the run's composite foreign key
		// (deleting an Enrollment finds its records by it) and the read of an Enrollment's records.
		index.Fields("workspace_id", "run_id", "step_id").
			Unique().
			StorageKey("automation_step_records_workspace_id_run_id_step_id"),
		// The records that keep a copy of an Event, by the Event's id: a refusal of tracking clears
		// the copies of the opens and clicks it deletes by their ids (ForgetKept, ADR 0136). Only a
		// record of a Wait for event that arrived on an Event and still runs holds one.
		index.Fields("workspace_id", "awaited_event_id").
			Annotations(entsql.IndexWhere("awaited_event_id IS NOT NULL")).
			StorageKey("automation_step_records_awaited_event_id"),
	}
}
