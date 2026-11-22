package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
)

// AutomationStepRecord is what an Enrollment keeps of one Step it reached (ADR 0124): when it
// reached and left it, how it left it and, at a wait, the moment the wait is due. One per
// Enrollment and Step: a version's Steps form a tree with no cycles. It is written in the
// transaction of the move that reaches or leaves the Step, which the Enrollment's own
// compare-and-set on its cursor decides, and goes with its Enrollment (ON DELETE CASCADE, so an
// Erasure of the Contact takes it). A send's own result stays on its Outbound message.
type AutomationStepRecord struct {
	ent.Schema
}

// Annotations adds to the enum checks that only a left Step record has an outcome (one the
// Enrollment ended on through an exit, a goal, a failure or an unsubscribe is left with none), and
// that only a skipped Step says why.
func (AutomationStepRecord) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("automation_step_records", AutomationStepRecord{}.Fields())
	a.Checks["automation_step_records_left_check"] = "outcome IS NULL OR left_at IS NOT NULL"
	a.Checks["automation_step_records_reason_check"] = "outcome IS NOT NULL OR skip_reason IS NULL"
	a.Checks["automation_step_records_skipped_check"] = variantCheck("outcome", stepOutcome)
	return []schema.Annotation{a}
}

// stepOutcome is the column that explains each outcome: a skipped Step its reason.
var stepOutcome = []variant{
	{value: "done"},
	{value: "skipped", required: []string{"skip_reason"}},
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
		// done: the Step did its work (a send's own result is its Outbound message's). skipped:
		// the Enrollment moved on without it, for skip_reason. Null on a Step the Enrollment ended
		// on through an exit, a goal, a failure or an unsubscribe: its end says why. Written in
		// the latch of left_at, never after it.
		field.Enum("outcome").
			Values("done", "skipped").
			Annotations(Latch()).
			Optional().
			Nillable(),
		// Why a skipped Step was skipped; the reasons are skipreason's, the one closed list (ADR
		// 0178).
		field.Enum("skip_reason").
			GoType(skipreason.Reason("")).
			Optional().
			Nillable(),
		// When a wait is due: computed once, when the Enrollment reached it, so a restart neither
		// moves it early nor makes it wait twice. Null for any other Step.
		field.Time("due_at").
			Annotations(Attribute("records the due moment a wait computed once when reached; the Enrollment's resume_at is what its job reads")).
			Optional().
			Nillable().
			Immutable(),
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
	}
}
