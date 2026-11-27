// Package automationsuse reads the use the Automations make of a row: the Definition use of a kept
// Automation's live version or draft that applies or removes a Tag, whose Trigger's, exit's or
// goal's filter, or a Wait for event's matcher filter, names a Tag or a Custom field (ADR 0133), or
// whose Branch's or Wait for event's Conditions test a Tag or a Custom field (ADR 0124), read as a
// Rule's Condition is read, and the Live use of the versions that will still send (ADR
// 0099). It is a reader of the Removal check
// (internal/domain/orchestration/removalcheck), the one package easement lets import it, so every
// removal that checks a row reads every holder, and which builds its Module. The automations
// module does not import it.
package automationsuse

import (
	"context"
	"slices"

	"github.com/getsphericon/sphericon/internal/kernel/tagname"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
)

// Module is the automations module's reader of the use of a row (docs/architecture/modules.md).
type Module struct{}

// New builds the reader. It needs nothing: every question reads in the scope it is given.
func New() *Module { return &Module{} }

// UsingTag returns the Workspace's kept Automations whose live version or draft applies or
// removes the Tag by its name, whose Trigger's, exit's, goal's or Wait for event's matcher filter
// names it (ADR 0133), or whose Branch's or Wait for event's Condition tests it or names it in an
// event test's filter (ADR 0124, ADR 0153), in id order (spec #767, "In use"). An archived
// Automation takes no part in new work.
func (m *Module) UsingTag(ctx context.Context, l *ent.WorkspaceLocked, name string) ([]*ent.Automation, error) {
	return using(ctx, l, func(v *ent.AutomationVersion) bool {
		return stepsTag(v, name) || slices.ContainsFunc(matcherNames(v).Tags, func(n string) bool { return tagname.Same(n, name) }) ||
			slices.ContainsFunc(stepConditions(v), func(g conditiontree.Group) bool { return condition.NamesTag(g, name) })
	})
}

// UsingCustomField returns the Workspace's kept Automations whose live version or draft holds the
// Custom field key against a retype to type t, in id order: its date Trigger or a Wait until waits
// on its date (ADR 0126), which only a datetime field holds; its Trigger's, exit's, goal's or Wait
// for event's matcher filter names it (ADR 0133), a contact.field_changed matcher that tests its
// `key`, which any retype breaks; or a Branch's or Wait for event's Condition tests it with a leaf
// a field of type t does not take (ADR 0124) or names it in an event test's filter (ADR 0153), the
// check a Rule's Condition holds a field by (condition.HoldsCustomField).
func (m *Module) UsingCustomField(ctx context.Context, l *ent.WorkspaceLocked, key string, t customfield.Type) ([]*ent.Automation, error) {
	return using(ctx, l, func(v *ent.AutomationVersion) bool {
		return datesField(v, key) || slices.Contains(matcherNames(v).CustomFields, key) ||
			slices.ContainsFunc(stepConditions(v), func(g conditiontree.Group) bool { return condition.HoldsCustomField(g, key, t) })
	})
}

// datesField reports whether the version waits on the date the Custom field key holds: its date
// Trigger names it, or a Wait until does, at any depth.
func datesField(v *ent.AutomationVersion, key string) bool {
	field := datetrigger.Field{Kind: datetrigger.CustomField, Key: key}
	switch t := automations.TriggerOf(v).(type) {
	case automations.DateTrigger:
		if t.Date.Field == field {
			return true
		}
	case automations.EventTrigger:
	}
	for _, d := range automationstep.OnDates(v.Steps) {
		if d.Field == field {
			return true
		}
	}
	return false
}

// stepConditions are the Conditions the version's Steps hold, at any depth: those of the paths of
// every Branch, and of every Wait for event (ADR 0124).
func stepConditions(v *ent.AutomationVersion) []conditiontree.Group {
	var out []conditiontree.Group
	for _, st := range automationstep.All(v.Steps) {
		switch p := st.Payload.(type) {
		case automationstep.Branch:
			for _, c := range p.Conditions() {
				out = append(out, c.Group)
			}
		case automationstep.WaitForEvent:
			if a, ok := p.Awaits.(automationstep.AwaitedEvent); ok && a.Condition != nil {
				out = append(out, a.Condition.Group)
			}
		}
	}
	return out
}

// using returns the Workspace's kept Automations whose live version or draft names holds of, in
// id order.
func using(ctx context.Context, l *ent.WorkspaceLocked, names func(*ent.AutomationVersion) bool) ([]*ent.Automation, error) {
	all, err := l.Automation().Query().WithLiveVersion().WithDraftVersion().Order(ent.Asc(automation.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var users []*ent.Automation
	for _, a := range all {
		live, draft := automations.LiveAndDraft(a)
		if (live != nil && names(live)) || (draft != nil && names(draft)) {
			users = append(users, a)
		}
	}
	return users, nil
}

// stepsTag reports whether a step of the version applies or removes the Tag.
func stepsTag(v *ent.AutomationVersion, name string) bool {
	for _, st := range automationstep.All(v.Steps) {
		if p, ok := st.Payload.(automationstep.Tagging); ok && tagname.Same(p.TagName(), name) {
			return true
		}
	}
	return false
}

// matcherNames is what the filters of the version's Event matchers name in the catalogue
// (condition.NamedDefinitions): its Trigger's, its exits', its goal's and those each of its Wait
// for events awaits.
func matcherNames(v *ent.AutomationVersion) condition.Definitions {
	var matchers []eventmatcher.Matcher
	switch t := automations.TriggerOf(v).(type) {
	case automations.EventTrigger:
		matchers = append(matchers, t.Matcher)
	case automations.DateTrigger:
	}
	matchers = append(matchers, automations.ExitsOf(v)...)
	if v.Goal != nil {
		matchers = append(matchers, *v.Goal)
	}
	for _, w := range automationstep.WaitsForEvents(v.Steps) {
		if a, ok := w.Awaits.(automationstep.AwaitedEvent); ok {
			matchers = append(matchers, a.Matchers...)
		}
	}
	var named condition.Definitions
	for _, m := range matchers {
		if m.Filter == nil {
			continue
		}
		d := condition.NamedDefinitions(m.Action, *m.Filter)
		named.Tags = append(named.Tags, d.Tags...)
		named.CustomFields = append(named.CustomFields, d.CustomFields...)
	}
	return named
}

// LiveSends returns the sends of the Automations in Live use (ADR 0099), in Automation id order:
// each send step of the live version of an active, kept Automation, and each send step of any
// version an unfinished Enrollment stands on, whatever its Automation's state, since the
// Enrollment still sends through it. A draft sends nothing until it is published (ADR 0065). An
// Automation with several send steps gives one Send per step; liveuse.SendUsers dedupes them for a
// refusal. A step carries no From of its own. A version that does not decode fails the read, so
// a removal never takes an unreadable definition for an unused one.
func (m *Module) LiveSends(ctx context.Context, l *ent.WorkspaceLocked) ([]liveuse.Send, error) {
	autos, err := sendingVersions(ctx, l)
	if err != nil {
		return nil, err
	}
	through := messaging.NewThroughs(l.Scope())
	var sends []liveuse.Send
	for _, a := range autos {
		user := liveuse.User{Kind: liveuse.Automation, ID: a.ID, Name: a.Name}
		for _, v := range a.Edges.Versions {
			for _, st := range automationstep.All(v.Steps) {
				ch, ok := st.Kind().Channel()
				if !ok {
					continue
				}
				integ, err := through.Of(ctx, ch, st.Integration())
				if err != nil {
					return nil, err
				}
				sends = append(sends, liveuse.Send{User: user, Channel: ch, Integration: integ})
			}
		}
	}
	return sends, nil
}

// sendingVersions returns the Automations with a version that still sends, in id order, each with
// those versions in Edges.Versions: the live version of an active, kept Automation, and every
// version an unfinished Enrollment stands on. An archived Automation is read too: its Enrollments
// still run.
func sendingVersions(ctx context.Context, l *ent.WorkspaceLocked) ([]*ent.Automation, error) {
	active, err := l.Automation().Query().
		Where(automation.StatusEQ(automation.StatusActive), automation.ArchivedAtIsNil(), automation.LiveVersionIDNotNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	live := lo.Map(active, func(a *ent.Automation, _ int) int64 { return *a.LiveVersionID })
	versions, err := l.AutomationVersion().Query().
		Where(automationversion.Or(
			automationversion.IDIn(live...),
			automationversion.HasRunsWith(automationrun.StatusEQ(automationrun.StatusActive)),
		)).
		Order(ent.Asc(automationversion.FieldID)).
		All(ctx)
	if err != nil || len(versions) == 0 {
		return nil, err
	}
	byAutomation := lo.GroupBy(versions, func(v *ent.AutomationVersion) int64 { return v.AutomationID })
	autos, err := l.Automation().QueryWithArchived().
		Where(automation.IDIn(lo.Keys(byAutomation)...)).
		Order(ent.Asc(automation.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range autos {
		a.Edges.Versions = byAutomation[a.ID]
	}
	return autos, nil
}
