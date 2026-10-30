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
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/inuse"
)

// EmailAutomations returns the active, unarchived Automations whose live version sends an email,
// as users. Such an Automation sends through the Integration its live version names (the
// Workspace's default email Integration when it names none) with its configured From, so that
// Integration and the Sending domain of that From are in use by it. A draft sends nothing until it
// is published, and Publish checks its Integration then (ADR 0065). The where predicates narrow
// the live versions further, for a caller that asks about one Integration.
func EmailAutomations(ctx context.Context, s *ent.Scoped, where ...predicate.AutomationVersion) ([]inuse.User, error) {
	autos, err := s.Automation().Query().
		Where(
			automation.StatusEQ(automation.StatusActive),
			automation.DiscardedAtIsNil(),
			automation.HasLiveVersionWith(where...),
		).
		WithLiveVersion().
		Order(ent.Asc(automation.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	var users []inuse.User
	for _, a := range autos {
		var steps []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(a.Edges.LiveVersion.Steps), &steps); err != nil {
			return nil, err
		}
		if slices.ContainsFunc(steps, func(st struct {
			Type string `json:"type"`
		}) bool {
			return st.Type == "email"
		}) {
			users = append(users, inuse.User{Kind: inuse.Automation, ID: a.ID, Name: a.Name})
		}
	}
	return users, nil
}
