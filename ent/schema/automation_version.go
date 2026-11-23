package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
)

// AutomationVersion is one copy of an Automation's definition: its Trigger, exit and goal matchers
// and steps (ADR 0049, ADR 0065, GLOSSARY: Automation version). Each send step names its own
// Integration inside the steps (ADR 0081). The Automation points at its live version
// and at its draft, the one version above live. An Enrollment begins on the live version and
// stays on it; only the draft is rewritten in place, and nothing ever stands on it. The name,
// status and archiving stay on the Automation.
type AutomationVersion struct {
	ent.Schema
}

func (AutomationVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "automation_versions", Checks: map[string]string{
			"automation_versions_steps_check":                  stepsCheck("steps"),
			"automation_versions_trigger_filter_check":         conditionCheck("trigger_filter"),
			"automation_versions_trigger_filter_entries_check": conditionEntriesCheck("trigger_filter"),
			"automation_versions_exits_check":                  jsonCheck("exits", "array"),
			"automation_versions_goal_check":                   jsonCheck("goal", "object"),
		}},
	}
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
		// The Event matchers that end an active Enrollment on this version early (`exited`), a
		// jsonb list (ADR 0169), and the one that ends it as having reached its goal
		// (`goal_reached`), a jsonb object, NULL for none (ADR 0133, eventmatcher.Matcher). Each is
		// an action and an optional filter on that Event's own properties; they may share an action
		// with the Trigger and with each other, and an Event matching the goal and an exit reaches
		// the goal.
		field.JSON("exits", []eventmatcher.Matcher{}).
			Default([]eventmatcher.Matcher{}).
			Annotations(entsql.Default("[]")),
		field.JSON("goal", &eventmatcher.Matcher{}).
			Optional(),
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
