// Package liveuse finds the live work that uses a row a removal is about to take away
// (ADR 0049), for the modules that cannot import internal/automations (it reaches them through
// outbound). It is a leaf over ent and the stored Step reader: each answer is the users to
// name in an *inuse.Error.
package liveuse

import (
	"context"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// SendingAutomations returns, as users, the active, unarchived Automations whose live version has a
// send step on the Channel for which through, given the Integration the step names (nil for the
// Channel's default), says yes. Such an Automation sends through that Integration whenever an
// Enrollment reaches the step (ADR 0081), so it is in use by it. A draft sends nothing until it is
// published, and Publish checks its steps' Integrations then (ADR 0065).
func SendingAutomations(ctx context.Context, s *ent.Scoped, ch messaging.Channel, through func(integrationID *int64) bool) ([]inuse.User, error) {
	autos, err := s.Automation().Query().
		Where(
			automation.StatusEQ(automation.StatusActive),
			automation.DiscardedAtIsNil(),
			automation.HasLiveVersion(),
		).
		WithLiveVersion().
		Order(ent.Asc(automation.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	var users []inuse.User
	for _, a := range autos {
		steps, err := automationsteps.Decode(a.Edges.LiveVersion.Steps)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(steps, func(st automationsteps.Step) bool {
			on, sends := st.Type.Channel()
			return sends && on == ch && through(st.IntegrationID)
		}) {
			users = append(users, inuse.User{Kind: inuse.Automation, ID: a.ID, Name: a.Name})
		}
	}
	return users, nil
}

// AnyIntegration is the through of SendingAutomations that takes every send step of the Channel.
func AnyIntegration(*int64) bool { return true }
