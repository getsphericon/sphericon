// Package workspacesummary is the use case of the Workspace summary (GLOSSARY, ADR 0111): a
// Workspace as its members and agents read it, built once below /site and /api, which map the
// same values into their own contracts. It reads identity (the Workspace and, for a User, the
// Memberships) and sending (the Holds on the Workspace, its Send cap with the last 24 hours of
// sends and the lengths a message is written against), so it is a use case, not a module of
// either context.
//
// What depends on the caller rather than the Workspace (the Role and its Permissions, the
// Second factor grace, which a strong session meets, and the Instance's own addresses) is the
// surface's mapping.
package workspacesummary

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackingdomains"
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

// Member is the summary of a Workspace a User is a member of, with the Membership (its User and
// Workspace loaded).
type Member struct {
	Membership *ent.Membership
	Summary    Summary
}

// UseCase reads the Workspace summary.
type UseCase struct {
	workspaces      *workspaces.Module
	memberships     *memberships.Module
	holds           *holds.Holds
	outbound        *outboundmessages.Module
	trackingDomains *trackingdomains.Module
}

// New builds the use case over the modules it reads.
func New(ws *workspaces.Module, ms *memberships.Module, hs *holds.Holds, out *outboundmessages.Module, td *trackingdomains.Module) *UseCase {
	return &UseCase{workspaces: ws, memberships: ms, holds: hs, outbound: out, trackingDomains: td}
}

// Of is the summary of the Workspace of the scope s.
func (u *UseCase) Of(ctx context.Context, s *ent.Scoped) (Summary, error) {
	ws, err := u.workspaces.Get(ctx, s)
	if err != nil {
		return Summary{}, err
	}
	return u.summarize(ctx, s, ws)
}

// OfMembers is the summary of every live Workspace the User is a member of, oldest Workspace
// first, each read in its own scope as the User.
func (u *UseCase) OfMembers(ctx context.Context, userID int64) ([]Member, error) {
	ms, err := u.memberships.OfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Member, len(ms))
	for i, m := range ms {
		sum, err := u.summarize(ctx, u.memberships.ScopeOf(m), m.Edges.Workspace)
		if err != nil {
			return nil, err
		}
		out[i] = Member{Membership: m, Summary: sum}
	}
	return out, nil
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
