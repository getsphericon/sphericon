package schema

import (
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// AutomationVersion is one copy of an Automation's definition: its Trigger, exit and goal events
// and steps (ADR 0049, ADR 0065, GLOSSARY: Automation version). Each send step names its own
// Integration inside the steps (ADR 0081). The Automation points at its live version
// and at its draft, the one version above live. An Enrollment begins on the live version and
// stays on it; only the draft is rewritten in place, and nothing ever stands on it. The name,
// status and archiving stay on the Automation.
type AutomationVersion struct {
	ent.Schema
}

// Annotations adds to the enum checks the shape of the steps and of the Trigger's filter, and the
// cooldown's floor (ADR 0127).
func (AutomationVersion) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("automation_versions", AutomationVersion{}.Fields())
	a.Checks["automation_versions_steps_check"] = stepsCheck("steps")
	a.Checks["automation_versions_trigger_filter_check"] = conditionCheck("trigger_filter")
	a.Checks["automation_versions_trigger_filter_entries_check"] = conditionEntriesCheck("trigger_filter")
	a.Checks["automation_versions_cooldown_check"] = fmt.Sprintf("cooldown_seconds >= %d", bounds.EntryCooldownMin)
	return []schema.Annotation{a}
}

func (AutomationVersion) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{}, WorkspaceMixin{Ref: "automation_versions"},
		VersionMixin{Owner: "automation", OwnerType: Automation.Type, Table: "automation_versions"},
	}
}

func (AutomationVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The steps (automationstep.Path), a jsonb object (ADR 0169): the root path of Steps
		// (ADR 0122) with the id every version keeps, {"id":<uuid>,"steps":[{"id":<uuid>,
		// "type":"email","subject":..,"body":<html>,"integrationId":N},{"id":<uuid>,"type":"wait",
		// "seconds":N}]}. A send step's integrationId is the Integration it sends through, absent for
		// its Channel's default (ADR 0081). Every version is written with its steps, so the column has
		// no default. Sensitive: HTTP steps carry sealed URL, header values and secret (ADR 0039). The
		// version is not audited itself: the Automation's own entries say it was archived or restored.
		field.JSON("steps", automationstep.Path{}).
			Annotations(Sensitive{}),
		// The version's Trigger is an Event matcher (ADR 0133, eventmatcher.Matcher): the
		// Event.action that enrolls a Contact (e.g. "contact.created", "email.opened", a custom
		// collect event), a column of its own for the hot lookup of the live versions an action
		// starts, and the filter on that Event's own properties (conditiontree.EventFilter), a
		// jsonb object (ADR 0169) that Postgres runs as one SQL/JSON path (ADR 0180). NULL is no
		// filter: every Event of the action enrolls.
		field.String("trigger_action"),
		field.JSON("trigger_filter", &conditiontree.EventFilter{}).
			Optional(),
		// The Event actions that end an active Enrollment on this version early (`exited`), and
		// the one that ends it as having reached its goal (`goal_reached`); none is the trigger.
		field.JSON("exit_events", []string{}).
			Default([]string{}).
			Annotations(entsql.Default("[]")),
		field.String("goal_event").
			Optional().
			Nillable(),
		// The Entry policy of the Event Trigger (ADR 0127): `once` enrolls a Contact in the
		// Automation at most once, ever; `again` enrolls it again once its previous Enrollment
		// ended and cooldown_seconds passed since that Enrollment began. The policy of the version
		// an Enrollment would begin on decides, for every way in.
		field.Enum("entry_policy").
			Values("once", "again").
			Default("once"),
		// The least time in seconds from the beginning of a Contact's previous Enrollment in the
		// Automation to the next under `again`: at least an hour, which is also its default.
		field.Int("cooldown_seconds").
			Default(bounds.TriggerCooldownDefault),
	}
}

func (AutomationVersion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("runs", AutomationRun.Type),
	}
}

func (AutomationVersion) Indexes() []ent.Index {
	return []ent.Index{
		// Trigger lookup: the live versions for a given event action.
		index.Fields("workspace_id", "trigger_action"),
	}
}
