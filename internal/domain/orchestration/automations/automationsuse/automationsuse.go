// Package automationsuse reads the use the Automations make of a row: the Definition use of a kept
// Automation's live version or draft that applies or removes a Tag, and the Live use of the
// versions that will still send (ADR 0099). It is a reader of the Removal check
// (internal/domain/orchestration/removalcheck), the one package easement lets import it, so every
// removal that checks a row reads every holder. The automations module does not import it.
package automationsuse

import (
	"context"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
)

// UsingTag returns the Workspace's kept Automations whose live version or draft applies or
// removes the Tag by its name, in id order (spec #767, "In use"). An archived Automation takes no
// part in new work.
func UsingTag(ctx context.Context, l *ent.WorkspaceLocked, name string) ([]*ent.Automation, error) {
	all, err := l.Automation().Query().WithLiveVersion().WithDraftVersion().Order(ent.Asc(automation.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var using []*ent.Automation
	for _, a := range all {
		live, draft := automations.LiveAndDraft(a)
		inLive, err := namesTag(live, name)
		if err != nil {
			return nil, err
		}
		inDraft, err := namesTag(draft, name)
		if err != nil {
			return nil, err
		}
		if inLive || inDraft {
			using = append(using, a)
		}
	}
	return using, nil
}

// namesTag reports whether a step of the version, if there is one, applies or removes the Tag.
func namesTag(v *ent.AutomationVersion, name string) (bool, error) {
	if v == nil {
		return false, nil
	}
	steps, err := automationstep.Decode(v.Steps)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(steps, func(st automationstep.Step) bool {
		return (st.Type == automationstep.ApplyTag || st.Type == automationstep.RemoveTag) && strings.TrimSpace(st.Tag) == name
	}), nil
}

// LiveSends returns the sends of the Automations in Live use (ADR 0099), in Automation id order:
// each send step of the live version of an active, kept Automation, and each send step of any
// version an unfinished Enrollment stands on, whatever its Automation's state, since the
// Enrollment still sends through it. A draft sends nothing until it is published (ADR 0065). An
// Automation with several send steps gives one Send per step; liveuse.SendUsers dedupes them for a
// refusal. A step carries no From of its own. A damaged stored definition is an error, so a
// removal never takes an unreadable definition for an unused one.
func LiveSends(ctx context.Context, l *ent.WorkspaceLocked) ([]liveuse.Send, error) {
	autos, err := sendingVersions(ctx, l)
	if err != nil {
		return nil, err
	}
	through := messaging.NewThroughs(l.Scope())
	var sends []liveuse.Send
	for _, a := range autos {
		user := liveuse.User{Kind: liveuse.Automation, ID: a.ID, Name: a.Name}
		for _, v := range a.Edges.Versions {
			steps, err := automationstep.Decode(v.Steps)
			if err != nil {
				return nil, err
			}
			for i := range steps {
				ch, ok := steps[i].Type.Channel()
				if !ok {
					continue
				}
				integ, err := through.Of(ctx, ch, steps[i].IntegrationID)
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
