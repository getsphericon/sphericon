// Package liveuse finds the live work that uses a row a removal is about to take away
// (ADR 0049), for the modules that cannot import internal/automations (it reaches them through
// outbound). It is a leaf over ent: each answer is the users to name in an *inuse.Error.
package liveuse

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/inuse"
)

// EmailAutomations returns the active, unarchived Automations whose current steps send an email, as users.
// Such an Automation sends through the Integration it names (the Workspace's default email
// Integration when it names none) with its configured From, so that Integration and the Sending
// domain of that From are in use by it. The where predicates narrow the Automations further, for
// a caller that asks about one Integration.
func EmailAutomations(ctx context.Context, s *ent.Scoped, where ...predicate.Automation) ([]inuse.User, error) {
	live := append([]predicate.Automation{automation.StatusEQ(automation.StatusActive), automation.DiscardedAtIsNil()}, where...)
	versions, err := s.AutomationVersion().Query().
		Where(automationversion.HasAutomationWith(live...)).
		WithAutomation().
		Order(ent.Asc(automationversion.FieldAutomationID), ent.Desc(automationversion.FieldVersion)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	var users []inuse.User
	seen := map[int64]bool{}
	for _, v := range versions {
		if seen[v.AutomationID] {
			continue // an older version: only the latest holds the current steps
		}
		seen[v.AutomationID] = true
		var steps []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(v.Steps), &steps); err != nil {
			return nil, err
		}
		if slices.ContainsFunc(steps, func(st struct {
			Type string `json:"type"`
		}) bool {
			return st.Type == "email"
		}) {
			users = append(users, inuse.User{Kind: inuse.Automation, ID: v.AutomationID, Name: v.Edges.Automation.Name})
		}
	}
	return users, nil
}
