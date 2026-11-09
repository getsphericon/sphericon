// Package workspaceview is the use case of the Workspace view (ADR 0111, amended by #1218): the
// operations of the Workspace screens, each of which answers with the Workspace summary (GLOSSARY)
// as the caller sees it. The summary is built once below /site and /api, which map the same values
// into their own contracts. It reads identity (the Workspace and, for a User, the Memberships) and
// sending (the Holds on the Workspace, its Send cap with the last 24 hours of sends and the lengths
// a message is written against), so it is a use case, not a module of either context.
//
// A member reads it as a View: the summary with their Membership and their grace under the
// Workspace's Two-factor requirement, which a strong session meets. Every operation of a member on
// a Workspace that answers with that View is here, since use cases do not import each other and a
// driver makes one call: the change in identity and the read of sending that follows it are one
// operation. Those are the changes of its settings, creating a Workspace (ADR 0043: the Workspace
// with its Owner Membership and the Send cap it starts with, ADR 0067) and restoring one inside its
// deletion window. What depends on the surface (the Permissions a Role maps to, the Instance's own
// addresses) is the surface's mapping.
//
// It takes the raw client and the bus's transactions: a Workspace being created belongs to no
// scope until it exists, and a deleted one has no scope its Owner can reach until it is restored.
package workspaceview

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackingdomains"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/smsrender"
)

// Summary is one Workspace's summary.
type Summary struct {
	Workspace *ent.Workspace
	// Holds are the Holds its owner sees on its sending: the pending deletion and the active Staff
	// holds (holds.Standing).
	Holds []*ent.Hold
	// SendCap is the Send cap the send path enforces with the sends of the last 24 hours (ADR
	// 0067); nil when none applies.
	SendCap *providerlimit.SendCapStatus
	// ShortLinkLength is how long a short tracked link of the Workspace is, 0 when the Instance
	// tracks no links; SMSMaxLength is the longest text an SMS goes out as. The SMS counter
	// counts against both.
	ShortLinkLength int
	SMSMaxLength    smsrender.Limit
	// TrackingTarget is the host a Tracking domain's CNAME points at, "" when the Instance has none.
	TrackingTarget string
}

// View is the summary of a Workspace as one of its members sees it.
type View struct {
	// Membership is the member's, its User loaded and its Workspace the one the summary read.
	Membership *ent.Membership
	Summary    Summary
	// SecondFactorGraceEndsAt is when the member's grace under the Workspace's Two-factor
	// requirement ends (ADR 0020), nil when none applies: the Workspace has no requirement, the
	// User has a Second factor, or the session is strong.
	SecondFactorGraceEndsAt *time.Time
}

// Caller is the member who reads or changes the Workspace of a scope, as their session presents
// them.
type Caller struct {
	// Membership is the caller's Membership of the Workspace, its User and Workspace loaded.
	Membership *ent.Membership
	// Actor is who the Audit log records for a change.
	Actor events.Actor
	// StrongSession is a session that passed a Second factor step or began with a Passkey (ADR
	// 0052): it meets the Two-factor requirement by itself, so it has no grace to count down, as
	// secondfactors.Module.Admit decides.
	StrongSession bool
}

// UpdateInput renames the Workspace and, when PostalAddress is set, sets its postal address (""
// clears it).
type UpdateInput struct {
	Name          string
	PostalAddress *string
}

// UseCase reads the Workspace view, and makes the changes of a Workspace that answer with it.
type UseCase struct {
	client          *ent.Client
	bus             *events.Bus
	workspaces      *workspaces.Module
	memberships     *memberships.Module
	secondFactors   *secondfactors.Module
	holds           *holds.Holds
	outbound        *outboundmessages.Module
	trackingDomains *trackingdomains.Module
	states          *instancestates.Module
	newPublicID     func() string
	now             func() time.Time
}

// Modules are the modules the use case reads and changes the Workspace through. States gives the
// Send cap a new Workspace starts with (instancestates.Module.StartingSendCapIn, ADR 0067); nil,
// as in a test, gives it none.
type Modules struct {
	Workspaces      *workspaces.Module
	Memberships     *memberships.Module
	SecondFactors   *secondfactors.Module
	Holds           *holds.Holds
	Outbound        *outboundmessages.Module
	TrackingDomains *trackingdomains.Module
	States          *instancestates.Module
}

// New builds the use case over the raw client a deleted Workspace is found with, the bus whose
// transactions create and restore one, and the modules it reads and changes; newPublicID draws a
// Workspace id (workspaceid.New; a test gives one that collides to watch workspacesunchecked.CreateIn
// draw again), and now is the Instance clock a Two-factor requirement starts at and a deletion
// window is measured on.
func New(client *ent.Client, bus *events.Bus, mods Modules, newPublicID func() string, now func() time.Time) *UseCase {
	return &UseCase{
		client: client, bus: bus, workspaces: mods.Workspaces, memberships: mods.Memberships, secondFactors: mods.SecondFactors,
		holds: mods.Holds, outbound: mods.Outbound, trackingDomains: mods.TrackingDomains, states: mods.States,
		newPublicID: newPublicID, now: now,
	}
}

// Of is the summary of the Workspace of the scope s.
func (u *UseCase) Of(ctx context.Context, s *ent.Scoped) (Summary, error) {
	ws, err := u.workspaces.Get(ctx, s)
	if err != nil {
		return Summary{}, err
	}
	return u.summarize(ctx, s, ws)
}

// OfMembers is the View of every live Workspace the User is a member of, oldest Workspace first,
// each read in its own scope as the User; strongSession is whether the User's session meets the
// Two-factor requirement by itself (Caller.StrongSession).
func (u *UseCase) OfMembers(ctx context.Context, userID int64, strongSession bool) ([]View, error) {
	ms, err := u.memberships.OfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	views := make([]View, len(ms))
	for i, m := range ms {
		sum, err := u.summarize(ctx, u.memberships.ScopeOf(m), m.Edges.Workspace)
		if err != nil {
			return nil, err
		}
		views[i] = View{Membership: m, Summary: sum}
	}
	if err := u.withGrace(ctx, views, strongSession); err != nil {
		return nil, err
	}
	return views, nil
}

// Update renames the Workspace of s and sets its postal address (workspaces.Module.Update) and
// answers its View for the caller.
func (u *UseCase) Update(ctx context.Context, s *ent.Scoped, c Caller, in UpdateInput) (View, error) {
	ws, err := u.workspaces.Update(ctx, s, c.Actor, in.Name, in.PostalAddress)
	if err != nil {
		return View{}, err
	}
	return u.view(ctx, s, c, ws)
}

// SetTimezone sets the time zone of the Workspace of s (workspaces.Module.SetTimezone, ADR 0057)
// and answers its View for the caller.
func (u *UseCase) SetTimezone(ctx context.Context, s *ent.Scoped, c Caller, zone string) (View, error) {
	ws, err := u.workspaces.SetTimezone(ctx, s, c.Actor, zone)
	if err != nil {
		return View{}, err
	}
	return u.view(ctx, s, c, ws)
}

// SetSecondFactorRequirement switches the Two-factor requirement of the Workspace of s on, from
// now, or off (workspaces.Module.SetSecondFactorRequirement, which tells the Users without a
// Second factor when it goes on) and answers its View for the caller, whose grace starts with it.
func (u *UseCase) SetSecondFactorRequirement(ctx context.Context, s *ent.Scoped, c Caller, required bool) (View, error) {
	ws, err := u.workspaces.SetSecondFactorRequirement(ctx, s, c.Actor, required, u.now())
	if err != nil {
		return View{}, err
	}
	return u.view(ctx, s, c, ws)
}

// view is the View of the Workspace ws of s for the caller, ws as it reads now: the caller's
// Membership takes it in place of the one it was loaded with, so the grace follows a change of
// the requirement.
func (u *UseCase) view(ctx context.Context, s *ent.Scoped, c Caller, ws *ent.Workspace) (View, error) {
	sum, err := u.summarize(ctx, s, ws)
	if err != nil {
		return View{}, err
	}
	m := *c.Membership
	m.Edges.Workspace = ws
	views := []View{{Membership: &m, Summary: sum}}
	if err := u.withGrace(ctx, views, c.StrongSession); err != nil {
		return View{}, err
	}
	return views[0], nil
}

// withGrace sets each View's grace end, read for all of them at once; a strong session has none.
func (u *UseCase) withGrace(ctx context.Context, views []View, strongSession bool) error {
	if strongSession || len(views) == 0 {
		return nil
	}
	ms := make([]*ent.Membership, len(views))
	for i, v := range views {
		ms[i] = v.Membership
	}
	ends, err := u.secondFactors.Deadlines(ctx, ms)
	if err != nil {
		return err
	}
	for i, v := range views {
		if end, ok := ends[v.Membership.ID]; ok {
			views[i].SecondFactorGraceEndsAt = &end
		}
	}
	return nil
}

// summarize reads the summary of the Workspace ws through s, its scope.
func (u *UseCase) summarize(ctx context.Context, s *ent.Scoped, ws *ent.Workspace) (Summary, error) {
	standing, err := u.holds.Standing(ctx, s)
	if err != nil {
		return Summary{}, err
	}
	sendCap, err := u.outbound.SendCapStatus(ctx, s, ws)
	if err != nil {
		return Summary{}, err
	}
	linkLength, err := u.outbound.ShortLinkLength(ctx, s)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		Workspace:       ws,
		Holds:           standing.Holds(),
		SendCap:         sendCap,
		ShortLinkLength: linkLength,
		SMSMaxLength:    u.outbound.SMSLengthLimit(),
		TrackingTarget:  u.trackingDomains.Target(),
	}, nil
}
