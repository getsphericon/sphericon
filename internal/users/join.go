package users

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/suspension"
)

// applyDomainJoin is the one place a Membership is created because an address was proven
// (ADR 0051). It runs on the transaction of the step that stamps email_proven_at from empty,
// so the Membership and the proof are atomic; it is called explicitly by each such step and
// is not an ent hook, because a hook would hide a Membership insert inside a User update.
//
// It reads the verified Join domain equal to the address's domain (never DNS: a domain whose
// record was lost has already been un-verified by the periodic re-check), builds the scope
// of the Workspace that row names, decides on the facts with DecideJoin and, on a join,
// creates a `member` Membership with source `domain` and records `membership.domain_join`
// with the system as the actor and the joiner as target. It returns the joined Workspace,
// nil when nothing was joined. Nothing is mailed.
func applyDomainJoin(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User) (*ent.Workspace, error) {
	_, host, ok := strings.Cut(u.Email, "@")
	if !ok {
		return nil, nil
	}
	domain, ok := domainname.Normalize(host)
	if !ok {
		return nil, nil
	}
	holder, err := joindomains.HolderIn(ctx, tx, domain)
	if err != nil || holder == nil {
		return nil, err
	}
	ws, err := tx.Workspace.Get(ctx, holder.WorkspaceID)
	if err != nil {
		return nil, err
	}

	// The scope of a Workspace the User has no session in: the closed list of docs/architecture.md names
	// this step, right after the Join domain row named the Workspace. It carries no actor, so
	// the Membership insert is not audited on its own; the entry below is the one record.
	ts := tx.Scoped(ws.ID)
	member, err := ts.Membership().Query().Where(membership.UserID(u.ID)).Exist(ctx)
	if err != nil {
		return nil, err
	}

	facts := JoinFacts{
		ProvenNow:     true,
		DomainHeld:    true,
		SwitchOn:      ws.DomainJoinEnabled,
		Suspended:     suspension.Is(ws),
		InDeletion:    ws.DiscardedAt != nil,
		HasMembership: member,
	}
	if DecideJoin(facts).Outcome != JoinAsMember {
		return nil, nil
	}

	joined, err := ts.Membership().Create().
		SetUserID(u.ID).
		SetRole(membership.RoleMember).
		SetSource(membership.SourceDomain).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	// A colleague admitted by their address is not a public sign-up: the Sign-up survey is never
	// asked of them, whichever path set the marker before the address was proven.
	if err := clearSurveyDue(ctx, tx, u.ID); err != nil {
		return nil, err
	}
	diff := map[string]any{
		"domain":             events.Became(domain),
		membership.FieldRole: events.Became(string(joined.Role)),
	}
	system := events.Actor{Kind: events.ActorSystem}
	if err := RecordActionOnUserIn(ctx, pub, system, u, events.ActionMembershipDomainJoin, diff, ws.ID); err != nil {
		return nil, err
	}
	return ws, nil
}
