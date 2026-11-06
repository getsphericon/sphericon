package automations

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/inuse"
)

// LiveSends returns the sends of the Automations in Live use (ADR 0099), in Automation id order:
// each send step of the live version of an active, kept Automation, and each send step of any
// version an unfinished Enrollment stands on, whatever its Automation's state, since the
// Enrollment still sends through it. A draft sends nothing until it is published (ADR 0065). An
// Automation with several send steps gives one Send per step; inuse.SendUsers dedupes them for a
// refusal. A step carries no From of its own. A damaged stored definition is an error, so a
// removal never takes an unreadable definition for an unused one.
func LiveSends(ctx context.Context, s *ent.Scoped) ([]inuse.Send, error) {
	autos, err := sendingVersions(ctx, s)
	if err != nil {
		return nil, err
	}
	through := messaging.NewThroughs(s)
	var sends []inuse.Send
	for _, a := range autos {
		user := inuse.User{Kind: inuse.Automation, ID: a.ID, Name: a.Name}
		for _, v := range a.Edges.Versions {
			steps, err := automationsteps.Decode(v.Steps)
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
				sends = append(sends, inuse.Send{User: user, Channel: ch, Integration: integ})
			}
		}
	}
	return sends, nil
}

// sendingVersions returns the Automations with a version that still sends, in id order, each with
// those versions in Edges.Versions: the live version of an active, kept Automation, and every
// version an unfinished Enrollment stands on. An archived Automation is read too: its Enrollments
// still run.
func sendingVersions(ctx context.Context, s *ent.Scoped) ([]*ent.Automation, error) {
	active, err := s.Automation().Query().
		Where(automation.StatusEQ(automation.StatusActive), automation.ArchivedAtIsNil(), automation.LiveVersionIDNotNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	live := lo.Map(active, func(a *ent.Automation, _ int) int64 { return *a.LiveVersionID })
	versions, err := s.AutomationVersion().Query().
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
	autos, err := s.Automation().QueryWithArchived().
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
