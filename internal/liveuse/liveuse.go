// Package liveuse finds the Live use that sends (ADR 0099): the work that will still send, and the
// route each of its sends takes, for the modules whose removal it blocks (Integrations, Sending
// domains) and that cannot import internal/automations (it reaches them through outbound). It is a
// leaf over ent, the stored Step reader and the Send route's own resolution of the Integration
// (messaging.Through), so the removal check and the send path agree on where work sends.
package liveuse

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// Send is one send of Live use: who sends, on which Channel, and the route it takes.
type Send struct {
	// User is the work that sends: a Broadcast, or an Automation for a send step of one of its
	// versions.
	User    inuse.User
	Channel messaging.Channel
	// Integration is the Integration the send goes through (messaging.Through): the one it names,
	// else its Channel's default, enabled or not. Nil when it names one that is gone or of another
	// Channel, or names none while its Channel has no default.
	Integration *ent.Integration
	// From is the send's own From address on email, "" when it has none: it then goes out with the
	// From of its Integration (messaging.EffectiveFrom). An Automation step carries none.
	From string
}

// Sends returns the sends of the Workspace's Live use (ADR 0099), Broadcasts first, then
// Automations, each in id order: a scheduled or sending Broadcast; each send step of the live
// version of an active, kept Automation; and each send step of any version an unfinished
// Enrollment stands on, whatever its Automation's state, since the Enrollment still sends through
// it. A draft sends nothing until it is scheduled or published (ADR 0065). An Automation with
// several send steps gives one Send per step; Users dedupes them for a refusal.
func Sends(ctx context.Context, s *ent.Scoped) ([]Send, error) {
	r := router{s: s, through: map[routeKey]*ent.Integration{}}
	live, err := s.Broadcast().Query().
		Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending)).
		Order(ent.Asc(broadcast.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	var sends []Send
	for _, b := range live {
		send, err := r.route(ctx, inuse.User{Kind: inuse.Broadcast, ID: b.ID, Name: b.Name},
			messaging.Channel(b.Channel), b.IntegrationID, lo.FromPtr(b.FromEmail))
		if err != nil {
			return nil, err
		}
		sends = append(sends, send)
	}
	autos, err := sendingVersions(ctx, s)
	if err != nil {
		return nil, err
	}
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
				send, err := r.route(ctx, user, ch, steps[i].IntegrationID, "")
				if err != nil {
					return nil, err
				}
				sends = append(sends, send)
			}
		}
	}
	return sends, nil
}

// Users is the work of the sends, each named once, in the order of sends.
func Users(sends []Send) []inuse.User {
	users := lo.Map(sends, func(send Send, _ int) inuse.User { return send.User })
	return lo.UniqBy(users, func(u inuse.User) inuse.User { return inuse.User{Kind: u.Kind, ID: u.ID} })
}

// sendingVersions returns the Automations with a version that still sends, in id order, each with
// those versions in Edges.Versions: the live version of an active, kept Automation, and every
// version an unfinished Enrollment stands on. An archived Automation is read too: its Enrollments
// still run.
func sendingVersions(ctx context.Context, s *ent.Scoped) ([]*ent.Automation, error) {
	active, err := s.Automation().Query().
		Where(automation.StatusEQ(automation.StatusActive), automation.DiscardedAtIsNil(), automation.LiveVersionIDNotNil()).
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
	autos, err := s.Automation().QueryWithDiscarded().
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

type routeKey struct {
	ch messaging.Channel
	id int64
}

// router resolves the Integration of each send once per (Channel, named Integration).
type router struct {
	s       *ent.Scoped
	through map[routeKey]*ent.Integration
}

func (r router) route(ctx context.Context, user inuse.User, ch messaging.Channel, id *int64, from string) (Send, error) {
	key := routeKey{ch, lo.FromPtr(id)}
	integ, seen := r.through[key]
	if !seen {
		var err error
		integ, err = messaging.Through(ctx, r.s, ch, id)
		if err != nil && !errors.Is(err, messaging.ErrNoProvider) {
			return Send{}, err
		}
		r.through[key] = integ
	}
	return Send{User: user, Channel: ch, Integration: integ, From: from}, nil
}

// Uses reports whether the send goes through the Integration with the id.
func (s Send) Uses(integrationID int64) bool {
	return s.Integration != nil && s.Integration.ID == integrationID
}
