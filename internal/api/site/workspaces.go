package site

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accountlink"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/suspension"
)

// SiteWorkspacesList returns the workspaces the authenticated user is a member of.
func (h *Handlers) SiteWorkspacesList(ctx context.Context) ([]siteapi.SiteWorkspaceResource, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return []siteapi.SiteWorkspaceResource{}, nil
	}

	items, err := h.accounts.MembershipsOf(ctx, a.UserID)
	if err != nil {
		return nil, err
	}

	resources := make([]siteapi.SiteWorkspaceResource, len(items))
	for i, m := range items {
		if resources[i], err = h.workspaceResource(ctx, h.accounts.ScopeOf(m), m); err != nil {
			return nil, err
		}
	}
	return resources, nil
}

// workspaceResource is the Membership's Workspace as the Membership's User sees it,
// with their grace end under the Two-factor requirement (ADR 0020) and the Staff member
// holds active now (ADR 0009), read through s, the Workspace's scope. The Membership's
// User and Workspace edges must be loaded.
func (h *Handlers) workspaceResource(ctx context.Context, s *ent.Scoped, m *ent.Membership) (siteapi.SiteWorkspaceResource, error) {
	r := mapper.WorkspaceToResource(m.Edges.Workspace)
	r.Role = siteapi.SiteMembershipRole(m.Role)
	if suspension.Is(m.Edges.Workspace) && h.supportEmail != "" {
		r.SupportEmail = siteapi.NewOptNilString(h.supportEmail)
	}
	r.McpUrl = h.origins.MCP
	r.TrackerUrl = h.origins.Tracker
	held := accounts.Permissions(m.Role)
	r.Permissions = lo.Map(held, func(p accounts.Permission, _ int) siteapi.Permission { return siteapi.Permission(p) })
	r.AccountLinkAvailable = h.accountLink != nil && slices.Contains(held, accounts.PermAccountLinkOpen)
	// A strong session (a Passkey login or a Second factor step) meets the requirement, so it has
	// no grace to count down: this is the decision secondfactor.Withheld makes.
	if end, ok := secondfactor.Deadline(m); ok && !currentSession(ctx).Method.Strong() {
		r.SecondFactorGraceEndsAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(end))
	}
	standing, err := h.holds.Standing(ctx, s)
	if err != nil {
		return r, err
	}
	r.Holds = lo.Map(standing.Holds(), func(hold *ent.Hold, _ int) siteapi.SiteWorkspaceHold {
		return mapper.HoldToResource(hold)
	})
	return r, nil
}

// SiteWorkspacesCreate creates a Workspace for the authenticated User, who becomes its
// Owner (ADR 0043). Only a Verified User may create one.
func (h *Handlers) SiteWorkspacesCreate(ctx context.Context, req *siteapi.SiteCreateWorkspaceInput) (siteapi.SiteWorkspacesCreateRes, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, &ent.NotFoundError{}
	}
	ws, err := h.accounts.CreateWorkspace(ctx, a.UserID, req.Name)
	if err != nil {
		return nil, err
	}
	s, m, err := h.accounts.Scope(ctx, a.UserID, ws.PublicID)
	if err != nil {
		return nil, err
	}
	r, err := h.workspaceResource(ctx, s, m)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SiteWorkspacesAccountLink mints the Owner's Account link token (Permission
// account_link:open, which Access checks first). Without the installation's
// configuration it answers 404.
func (h *Handlers) SiteWorkspacesAccountLink(ctx context.Context, _ siteapi.SiteWorkspacesAccountLinkParams) (siteapi.SiteWorkspacesAccountLinkRes, error) {
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
func (h *Handlers) SiteWorkspacesRequireSecondFactor(ctx context.Context, _ siteapi.SiteWorkspacesRequireSecondFactorParams) (siteapi.SiteWorkspacesRequireSecondFactorRes, error) {
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
func (h *Handlers) SiteWorkspacesLiftSecondFactorRequirement(ctx context.Context, _ siteapi.SiteWorkspacesLiftSecondFactorRequirementParams) (siteapi.SiteWorkspacesLiftSecondFactorRequirementRes, error) {
	r, err := h.switchSecondFactorRequirement(ctx, false)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// switchSecondFactorRequirement switches the requirement of the Workspace the operation addresses
// on or off and answers its Workspace as the caller now sees it. Switching it on emails the Users
// without a Second factor.
func (h *Handlers) switchSecondFactorRequirement(ctx context.Context, required bool) (siteapi.SiteWorkspaceResource, error) {
	s, m := workspaceScope(ctx), callerMembership(ctx)
	w, switchedOn, err := h.accounts.SetSecondFactorRequirement(ctx, s, h.actor(ctx), required, h.now())
	if err != nil {
		return siteapi.SiteWorkspaceResource{}, err
	}
	// The requirement is committed; the email is best-effort, like the invite.
	if switchedOn {
		if merr := h.sysmail.EnqueueSecondFactorRequired(ctx, s.WorkspaceID()); merr != nil {
			slog.WarnContext(ctx, "second factor required email not enqueued", "error", merr, "workspace_id", s.WorkspaceID())
		}
	}
	return h.workspaceResource(ctx, s, withWorkspace(m, w))
}

// SiteWorkspacesUpdate renames a workspace (Owner and Admin only). The id never changes.
func (h *Handlers) SiteWorkspacesUpdate(ctx context.Context, req *siteapi.SiteUpdateWorkspaceInput, _ siteapi.SiteWorkspacesUpdateParams) (siteapi.SiteWorkspacesUpdateRes, error) {
	s, m := workspaceScope(ctx), callerMembership(ctx)

	var postal *string
	// postalAddress is optional in the contract: absent = leave unchanged, present
	// (incl. empty string) = set/clear. Trimmed so a whitespace-only value clears it.
	if req.PostalAddress.Set {
		v := strings.TrimSpace(req.PostalAddress.Value)
		postal = &v
	}
	w, err := h.accounts.UpdateWorkspace(ctx, s, h.actor(ctx), req.Name, postal)
	if err != nil {
		return nil, err
	}
	resource, err := h.workspaceResource(ctx, s, withWorkspace(m, w))
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

// SiteWorkspacesSetTimezone sets the Workspace time zone (Permission workspace:timezone, ADR 0057).
func (h *Handlers) SiteWorkspacesSetTimezone(ctx context.Context, req *siteapi.SiteWorkspaceTimezoneInput, _ siteapi.SiteWorkspacesSetTimezoneParams) (siteapi.SiteWorkspacesSetTimezoneRes, error) {
	s, m := workspaceScope(ctx), callerMembership(ctx)
	w, err := h.accounts.SetTimezone(ctx, s, h.actor(ctx), string(req.Timezone))
	if err != nil {
		return nil, err
	}
	resource, err := h.workspaceResource(ctx, s, withWorkspace(m, w))
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
	items, err := h.deletion.Deleted(ctx, a.UserID, h.now())
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
func (h *Handlers) SiteWorkspacesDelete(ctx context.Context, req *siteapi.SiteDeleteWorkspaceInput, _ siteapi.SiteWorkspacesDeleteParams) (siteapi.SiteWorkspacesDeleteRes, error) {
	s := workspaceScope(ctx)
	w, err := h.deletion.Delete(ctx, s, h.actor(ctx), req.Name, h.now())
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
func (h *Handlers) SiteWorkspacesLeave(ctx context.Context, _ siteapi.SiteWorkspacesLeaveParams) (siteapi.SiteWorkspacesLeaveRes, error) {
	s := workspaceScope(ctx)
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.accounts.Leave(ctx, s, userID); err != nil {
		return nil, err
	}
	return &siteapi.SiteWorkspacesLeaveNoContent{}, nil
}

// SiteWorkspacesRestore restores a Workspace in its deletion window. Its path names no workspaceId
// (a deleted Workspace has no live Membership for Access to load): the module restores it only for
// one of its Owners, the holders of workspace:delete, and answers 404 to anyone else.
func (h *Handlers) SiteWorkspacesRestore(ctx context.Context, params siteapi.SiteWorkspacesRestoreParams) (siteapi.SiteWorkspacesRestoreRes, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, accounts.ErrWorkspaceNotFound
	}
	if _, err := h.deletion.Restore(ctx, a.UserID, string(params.ID), h.actor(ctx), h.now()); err != nil {
		return nil, err
	}
	// The Workspace is live again, so the caller's Owner Membership resolves like any other.
	s, m, err := h.accounts.Scope(ctx, a.UserID, string(params.ID))
	if err != nil {
		return nil, err
	}
	r, err := h.workspaceResource(ctx, s, m)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SiteWorkspacesGetRevenueSettings reads the Reporting currency and the Attribution window
// (every member, ADR 0064).
func (h *Handlers) SiteWorkspacesGetRevenueSettings(ctx context.Context, _ siteapi.SiteWorkspacesGetRevenueSettingsParams) (siteapi.SiteWorkspacesGetRevenueSettingsRes, error) {
	s := workspaceScope(ctx)
	w, err := h.accounts.Workspace(ctx, s)
	if err != nil {
		return nil, err
	}
	r := mapper.RevenueSettingsToResource(w)
	return &r, nil
}

// SiteWorkspacesUpdateRevenueSettings changes the Reporting currency and the Attribution window
// (Owner and Admin, Permission revenue:configure, ADR 0064).
func (h *Handlers) SiteWorkspacesUpdateRevenueSettings(ctx context.Context, req *siteapi.SiteUpdateRevenueSettingsInput, _ siteapi.SiteWorkspacesUpdateRevenueSettingsParams) (siteapi.SiteWorkspacesUpdateRevenueSettingsRes, error) {
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
	w, err := h.accounts.SetRevenueSettings(ctx, s, h.actor(ctx), code, days)
	if err != nil {
		return nil, err
	}
	r := mapper.RevenueSettingsToResource(w)
	return &r, nil
}
