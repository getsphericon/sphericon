package site

import (
	"context"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/accountlink"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/usecase/workspacedeletion"
	"github.com/getsphericon/sphericon/internal/usecase/workspaceview"
)

// SiteWorkspacesList returns the workspaces the authenticated user is a member of.
func (h *Handlers) SiteWorkspacesList(ctx context.Context) ([]siteapi.SiteWorkspaceResource, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return []siteapi.SiteWorkspaceResource{}, nil
	}
	views, err := h.workspaceView.OfMembers(ctx, a.User.ID, a.Method.Strong())
	if err != nil {
		return nil, err
	}
	return lo.Map(views, func(v workspaceview.View, _ int) siteapi.SiteWorkspaceResource { return h.workspaceResource(v) }), nil
}

// viewCaller is the caller of an operation on the Workspace it addresses, as the Workspace
// view takes them: their Membership, the actor and whether their session is strong.
func (h *Handlers) viewCaller(ctx context.Context) workspaceview.Caller {
	return workspaceview.Caller{Membership: callerMembership(ctx), Actor: h.actor(ctx), StrongSession: currentSession(ctx).Method.Strong()}
}

// workspaceResource maps the Workspace summary as the member of the View sees it, with their Role
// and Permissions, their grace end under the Two-factor requirement (ADR 0020) and the Instance's
// addresses besides the app's own.
func (h *Handlers) workspaceResource(v workspaceview.View) siteapi.SiteWorkspaceResource {
	sum := v.Summary
	r := mapper.WorkspaceToResource(sum.Workspace)
	r.Role = mapper.MembershipRoleToResource(v.Membership.Role)
	if suspension.Is(sum.Workspace) && h.supportEmail != "" {
		r.SupportEmail = siteapi.NewOptNilString(h.supportEmail)
	}
	r.McpUrl = h.origins.MCP
	r.TrackerUrl = h.origins.Tracker
	if sum.TrackingTarget != "" {
		r.TrackingTarget = siteapi.NewOptString(sum.TrackingTarget)
	}
	held := memberships.Permissions(v.Membership.Role)
	r.Permissions = mapper.PermissionsToResources(held)
	r.AccountLinkAvailable = h.accountLink != nil && slices.Contains(held, memberships.PermAccountLinkOpen)
	if end := v.SecondFactorGraceEndsAt; end != nil {
		r.SecondFactorGraceEndsAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(*end))
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
	in := inputs.WorkspaceCreate(req)
	in.UserID, in.StrongSession = a.User.ID, a.Method.Strong()
	v, err := h.workspaceView.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	r := h.workspaceResource(v)
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
	v, err := h.workspaceView.SetSecondFactorRequirement(ctx, workspaceScope(ctx), h.viewCaller(ctx), required)
	if err != nil {
		return siteapi.SiteWorkspaceResource{}, err
	}
	return h.workspaceResource(v), nil
}

// SiteWorkspacesUpdate renames a workspace (Owner and Admin only). The id never changes.
func (h *Handlers) SiteWorkspacesUpdate(ctx context.Context, req *siteapi.SiteUpdateWorkspaceInput, _ siteapi.SiteWorkspacesUpdateParams) (*siteapi.SiteWorkspaceResource, error) {
	v, err := h.workspaceView.Update(ctx, workspaceScope(ctx), h.viewCaller(ctx), inputs.WorkspaceUpdate(req))
	if err != nil {
		return nil, err
	}
	resource := h.workspaceResource(v)
	return &resource, nil
}

// SiteWorkspacesSetTimezone sets the Workspace time zone (Permission workspace:timezone, ADR 0057).
func (h *Handlers) SiteWorkspacesSetTimezone(ctx context.Context, req *siteapi.SiteWorkspaceTimezoneInput, _ siteapi.SiteWorkspacesSetTimezoneParams) (*siteapi.SiteWorkspaceResource, error) {
	v, err := h.workspaceView.SetTimezone(ctx, workspaceScope(ctx), h.viewCaller(ctx), string(req.Timezone))
	if err != nil {
		return nil, err
	}
	resource := h.workspaceResource(v)
	return &resource, nil
}

// deletedWorkspaceResource is a Workspace in its deletion window with the date its data
// will be erased.
func deletedWorkspaceResource(d workspacedeletion.DeletedWorkspace) siteapi.SiteDeletedWorkspaceResource {
	res := mapper.DeletedWorkspaceToResource(d.Workspace)
	res.OnlyOwner = d.OnlyOwner
	return res
}

// SiteWorkspacesListDeleted lists the Workspaces in their deletion window where the caller's Role
// holds workspace:delete.
func (h *Handlers) SiteWorkspacesListDeleted(ctx context.Context) ([]siteapi.SiteDeletedWorkspaceResource, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return []siteapi.SiteDeletedWorkspaceResource{}, nil
	}
	items, err := h.deletion.Deleted(ctx, a.User.ID)
	if err != nil {
		return nil, err
	}
	return lo.Map(items, func(d workspacedeletion.DeletedWorkspace, _ int) siteapi.SiteDeletedWorkspaceResource {
		return deletedWorkspaceResource(d)
	}), nil
}

// SiteWorkspacesDelete deletes a Workspace (Owner only) after the typed name matches.
func (h *Handlers) SiteWorkspacesDelete(ctx context.Context, req *siteapi.SiteDeleteWorkspaceInput, _ siteapi.SiteWorkspacesDeleteParams) (*siteapi.SiteDeletedWorkspaceResource, error) {
	d, err := h.deletion.Delete(ctx, workspaceScope(ctx), h.actor(ctx), req.Name)
	if err != nil {
		return nil, err
	}
	r := deletedWorkspaceResource(d)
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
	in.UserID, in.Actor, in.StrongSession = a.User.ID, h.actor(ctx), a.Method.Strong()
	v, err := h.workspaceView.Restore(ctx, in)
	if err != nil {
		return nil, err
	}
	r := h.workspaceResource(v)
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
