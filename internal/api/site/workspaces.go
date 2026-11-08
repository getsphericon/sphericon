package site

import (
	"context"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/accountlink"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/usecase/workspacesummary"
)

// SiteWorkspacesList returns the workspaces the authenticated user is a member of.
func (h *Handlers) SiteWorkspacesList(ctx context.Context) ([]siteapi.SiteWorkspaceResource, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return []siteapi.SiteWorkspaceResource{}, nil
	}
	members, err := h.workspaceSummary.OfMembers(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return lo.Map(members, func(m workspacesummary.Member, _ int) siteapi.SiteWorkspaceResource {
		return h.workspaceResource(ctx, m.Membership, m.Summary)
	}), nil
}

// summarizedWorkspace is the Workspace of s, whose Membership m the caller holds, as the
// Membership's User sees it after a change of it.
func (h *Handlers) summarizedWorkspace(ctx context.Context, s *ent.Scoped, m *ent.Membership) (siteapi.SiteWorkspaceResource, error) {
	sum, err := h.workspaceSummary.Of(ctx, s)
	if err != nil {
		return siteapi.SiteWorkspaceResource{}, err
	}
	return h.workspaceResource(ctx, m, sum), nil
}

// workspaceResource maps the Workspace summary as the Membership's User sees it, with their Role
// and Permissions, their grace end under the Two-factor requirement (ADR 0020) and the Instance's
// addresses besides the app's own. The Membership's User edge must be loaded.
func (h *Handlers) workspaceResource(ctx context.Context, m *ent.Membership, sum workspacesummary.Summary) siteapi.SiteWorkspaceResource {
	m = withWorkspace(m, sum.Workspace)
	r := mapper.WorkspaceToResource(sum.Workspace)
	r.Role = mapper.MembershipRoleToResource(m.Role)
	if suspension.Is(sum.Workspace) && h.supportEmail != "" {
		r.SupportEmail = siteapi.NewOptNilString(h.supportEmail)
	}
	r.McpUrl = h.origins.MCP
	r.TrackerUrl = h.origins.Tracker
	if sum.TrackingTarget != "" {
		r.TrackingTarget = siteapi.NewOptString(sum.TrackingTarget)
	}
	held := memberships.Permissions(m.Role)
	r.Permissions = mapper.PermissionsToResources(held)
	r.AccountLinkAvailable = h.accountLink != nil && slices.Contains(held, memberships.PermAccountLinkOpen)
	// A strong session (a Passkey login or a Second factor step) meets the requirement, so it has
	// no grace to count down: this is the decision secondfactors.Module.Admit makes.
	if !currentSession(ctx).Method.Strong() {
		end, ok, err := h.secondFactor.Deadline(ctx, m)
		if err != nil {
			return r, err
		}
		if ok {
			r.SecondFactorGraceEndsAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(end))
		}
	}
	r.Holds = mapper.HoldsToResources(sum.Holds)
	r.SendCap = mapper.SendCapStatusToResource(sum.SendCap)
	r.ShortLinkLength = narrow.Int32(sum.ShortLinkLength)
	r.SmsMaxLength = mapper.SmsLengthLimitToResource(sum.SMSMaxLength)
	return r
}

// SiteWorkspacesCreate creates a Workspace for the authenticated User, who becomes its
// Owner (ADR 0043). Only a Verified User may create one.
func (h *Handlers) SiteWorkspacesCreate(ctx context.Context, req *siteapi.SiteCreateWorkspaceInput) (*siteapi.SiteWorkspaceResource, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, &ent.NotFoundError{}
	}
	ws, err := h.workspaceCreation.CreateWorkspace(ctx, a.UserID, req.Name)
	if err != nil {
		return nil, err
	}
	s, m, err := h.memberships.Scope(ctx, a.UserID, ws.PublicID)
	if err != nil {
		return nil, err
	}
	r, err := h.summarizedWorkspace(ctx, s, m)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SiteWorkspacesAccountLink mints the Owner's Account link token (Permission
// account_link:open, which Access checks first). Without the installation's
// configuration it answers 404.
func (h *Handlers) SiteWorkspacesAccountLink(ctx context.Context, _ siteapi.SiteWorkspacesAccountLinkParams) (*siteapi.SiteAccountLinkResource, error) {
	if h.accountLink == nil {
		return nil, accountlink.ErrNotConfigured
	}
	s, m := workspaceScope(ctx), callerMembership(ctx)
	token, expires, err := h.accountLink.Token(accountlink.Subject{UserID: m.UserID, WorkspaceID: s.WorkspaceID(), Role: string(m.Role)}, h.now())
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAccountLinkResource{URL: h.accountLink.Address(), Token: token, ExpiresAt: siteapi.Timestamp(expires)}, nil
}

// withWorkspace is the Membership with its Workspace replaced by w (after an update).
func withWorkspace(m *ent.Membership, w *ent.Workspace) *ent.Membership {
	c := *m
	c.Edges.Workspace = w
	return &c
}

// SiteWorkspacesRequireSecondFactor switches the Workspace's Two-factor requirement on (Permission
// second_factor:manage).
func (h *Handlers) SiteWorkspacesRequireSecondFactor(ctx context.Context, _ siteapi.SiteWorkspacesRequireSecondFactorParams) (*siteapi.SiteWorkspaceResource, error) {
	r, err := h.switchSecondFactorRequirement(ctx, true)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SiteWorkspacesLiftSecondFactorRequirement switches the Workspace's Two-factor requirement off
// (Permission second_factor:manage). It is the one operation a User withheld by the requirement
// may still perform in the Workspace, as the contract declares, so an Owner or Admin without a
// Second factor is never locked out of lifting it.
func (h *Handlers) SiteWorkspacesLiftSecondFactorRequirement(ctx context.Context, _ siteapi.SiteWorkspacesLiftSecondFactorRequirementParams) (*siteapi.SiteWorkspaceResource, error) {
	r, err := h.switchSecondFactorRequirement(ctx, false)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// switchSecondFactorRequirement switches the requirement of the Workspace the operation addresses
// on or off and answers its Workspace as the caller now sees it. Switching it on tells the Users
// without a Second factor (workspaces.Module.SetSecondFactorRequirement publishes it).
func (h *Handlers) switchSecondFactorRequirement(ctx context.Context, required bool) (siteapi.SiteWorkspaceResource, error) {
	s, m := workspaceScope(ctx), callerMembership(ctx)
	if _, err := h.workspaces.SetSecondFactorRequirement(ctx, s, h.actor(ctx), required, h.now()); err != nil {
		return siteapi.SiteWorkspaceResource{}, err
	}
	return h.summarizedWorkspace(ctx, s, m)
}

// SiteWorkspacesUpdate renames a workspace (Owner and Admin only). The id never changes.
func (h *Handlers) SiteWorkspacesUpdate(ctx context.Context, req *siteapi.SiteUpdateWorkspaceInput, _ siteapi.SiteWorkspacesUpdateParams) (*siteapi.SiteWorkspaceResource, error) {
	s, m := workspaceScope(ctx), callerMembership(ctx)

	var postal *string
	// postalAddress is optional in the contract: absent = leave unchanged, present
	// (incl. empty string) = set/clear. Trimmed so a whitespace-only value clears it.
	if req.PostalAddress.Set {
		v := strings.TrimSpace(req.PostalAddress.Value)
		postal = &v
	}
	if _, err := h.workspaces.Update(ctx, s, h.actor(ctx), req.Name, postal); err != nil {
		return nil, err
	}
	resource, err := h.summarizedWorkspace(ctx, s, m)
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

// SiteWorkspacesSetTimezone sets the Workspace time zone (Permission workspace:timezone, ADR 0057).
func (h *Handlers) SiteWorkspacesSetTimezone(ctx context.Context, req *siteapi.SiteWorkspaceTimezoneInput, _ siteapi.SiteWorkspacesSetTimezoneParams) (*siteapi.SiteWorkspaceResource, error) {
	s, m := workspaceScope(ctx), callerMembership(ctx)
	if _, err := h.workspaces.SetTimezone(ctx, s, h.actor(ctx), string(req.Timezone)); err != nil {
		return nil, err
	}
	resource, err := h.summarizedWorkspace(ctx, s, m)
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

// deletedWorkspaceResource is a Workspace in its deletion window with the date its data
// will be erased.
func deletedWorkspaceResource(w *ent.Workspace, onlyOwner bool) siteapi.SiteDeletedWorkspaceResource {
	res := mapper.DeletedWorkspaceToResource(w)
	res.OnlyOwner = onlyOwner
	return res
}

// SiteWorkspacesListDeleted lists the Workspaces in their deletion window where the caller's Role
// holds workspace:delete.
func (h *Handlers) SiteWorkspacesListDeleted(ctx context.Context) ([]siteapi.SiteDeletedWorkspaceResource, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return []siteapi.SiteDeletedWorkspaceResource{}, nil
	}
	items, err := h.deletion.Deleted(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	sole, err := h.deletion.SoleOwned(ctx, lo.Map(items, func(m *ent.Membership, _ int) int64 { return m.WorkspaceID }))
	if err != nil {
		return nil, err
	}
	return lo.Map(items, func(m *ent.Membership, _ int) siteapi.SiteDeletedWorkspaceResource {
		return deletedWorkspaceResource(m.Edges.Workspace, sole[m.WorkspaceID])
	}), nil
}

// SiteWorkspacesDelete deletes a Workspace (Owner only) after the typed name matches.
func (h *Handlers) SiteWorkspacesDelete(ctx context.Context, req *siteapi.SiteDeleteWorkspaceInput, _ siteapi.SiteWorkspacesDeleteParams) (*siteapi.SiteDeletedWorkspaceResource, error) {
	s := workspaceScope(ctx)
	w, err := h.deletion.Delete(ctx, s, h.actor(ctx), req.Name)
	if err != nil {
		return nil, err
	}
	sole, err := h.deletion.SoleOwned(ctx, []int64{w.ID})
	if err != nil {
		return nil, err
	}
	r := deletedWorkspaceResource(w, sole[w.ID])
	return &r, nil
}

// SiteWorkspacesLeave removes the caller's own Membership (Leaving, ADR 0050). Any Role may
// leave; the only Owner is refused.
func (h *Handlers) SiteWorkspacesLeave(ctx context.Context, _ siteapi.SiteWorkspacesLeaveParams) error {
	s := workspaceScope(ctx)
	userID, err := sessionUserID(ctx)
	if err != nil {
		return err
	}
	return h.memberships.Leave(ctx, s, userID)
}

// SiteWorkspacesRestore restores a Workspace in its deletion window. Its path names no workspaceId
// (a deleted Workspace has no live Membership for Access to load): the module restores it only for
// one of its Owners, the holders of workspace:delete, and answers 404 to anyone else.
func (h *Handlers) SiteWorkspacesRestore(ctx context.Context, params siteapi.SiteWorkspacesRestoreParams) (*siteapi.SiteWorkspaceResource, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, workspaces.ErrNotFound
	}
	in := inputs.WorkspaceRestore(params)
	in.UserID, in.Actor = a.UserID, h.actor(ctx)
	if _, err := h.deletion.Restore(ctx, in); err != nil {
		return nil, err
	}
	// The Workspace is live again, so the caller's Owner Membership resolves like any other.
	s, m, err := h.memberships.Scope(ctx, a.UserID, string(params.ID))
	if err != nil {
		return nil, err
	}
	r, err := h.summarizedWorkspace(ctx, s, m)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SiteWorkspacesGetRevenueSettings reads the Reporting currency and the Attribution window
// (every member, ADR 0064).
func (h *Handlers) SiteWorkspacesGetRevenueSettings(ctx context.Context, _ siteapi.SiteWorkspacesGetRevenueSettingsParams) (*siteapi.SiteRevenueSettingsResource, error) {
	s := workspaceScope(ctx)
	w, err := h.workspaces.Get(ctx, s)
	if err != nil {
		return nil, err
	}
	r := mapper.RevenueSettingsToResource(w)
	return &r, nil
}

// SiteWorkspacesUpdateRevenueSettings changes the Reporting currency and the Attribution window
// (Owner and Admin, Permission revenue:configure, ADR 0064).
func (h *Handlers) SiteWorkspacesUpdateRevenueSettings(ctx context.Context, req *siteapi.SiteUpdateRevenueSettingsInput, _ siteapi.SiteWorkspacesUpdateRevenueSettingsParams) (*siteapi.SiteRevenueSettingsResource, error) {
	s := workspaceScope(ctx)
	var code *string
	if c, ok := req.ReportingCurrency.Get(); ok {
		str := string(c)
		code = &str
	}
	var days *int
	if d, ok := req.AttributionWindowDays.Get(); ok {
		n := int(d)
		days = &n
	}
	w, err := h.workspaces.SetRevenueSettings(ctx, s, h.actor(ctx), code, days)
	if err != nil {
		return nil, err
	}
	r := mapper.RevenueSettingsToResource(w)
	return &r, nil
}
