package site

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accountlink"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/secondfactor"
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
// with their grace end under the Two-factor requirement (ADR 0020) and the Operator
// holds active now (ADR 0009), read through s, the Workspace's scope. The Membership's
// User and Workspace edges must be loaded.
func (h *Handlers) workspaceResource(ctx context.Context, s *ent.Scoped, m *ent.Membership) (siteapi.SiteWorkspaceResource, error) {
	r := mapper.WorkspaceToResource(m.Edges.Workspace)
	r.Role = siteapi.SiteMembershipRole(m.Role)
	if m.Edges.Workspace.SuspendedAt != nil && h.supportEmail != "" {
		r.SupportEmail = siteapi.NewOptNilString(h.supportEmail)
	}
	r.AccountLinkAvailable = h.accountLink != nil && m.Role == membership.RoleOwner
	if end, ok := secondfactor.Deadline(m); ok {
		r.SecondFactorGraceEndsAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(end))
	}
	active, err := h.activeHolds(ctx, s, m.Edges.Workspace)
	if err != nil {
		return r, err
	}
	r.Holds = lo.Map(active, func(hold *ent.Hold, _ int) siteapi.SiteWorkspaceHold {
		return siteapi.SiteWorkspaceHold{Key: hold.Key, ReasonCode: hold.ReasonCode, Message: hold.Message, Since: siteapi.Timestamp(hold.CreatedAt), Billing: holds.IsBilling(hold.ReasonCode)}
	})
	return r, nil
}

// activeHolds is the holds the send path honours now: none without the `operator`
// license. A Workspace loaded with its holds edge is read from it; otherwise one query.
func (h *Handlers) activeHolds(ctx context.Context, s *ent.Scoped, w *ent.Workspace) ([]*ent.Hold, error) {
	if h.holdsHonoured != nil && !h.holdsHonoured() {
		return nil, nil
	}
	if loaded, err := w.Edges.HoldsOrErr(); err == nil {
		return holds.Live(loaded, h.now()), nil
	}
	return holds.Active(ctx, s, h.now())
}

// SiteWorkspacesAccountLink mints the Owner's Account link token. Without the
// installation's configuration it answers 404 before anything else; any role but
// Owner is refused.
func (h *Handlers) SiteWorkspacesAccountLink(ctx context.Context, params siteapi.SiteWorkspacesAccountLinkParams) (siteapi.SiteWorkspacesAccountLinkRes, error) {
	if h.accountLink == nil {
		v := siteapi.SiteWorkspacesAccountLinkNotFound(problem(http.StatusNotFound, "account link is not configured"))
		return &v, nil
	}
	s, m, err := h.membershipFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesAccountLinkNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if m.Role != membership.RoleOwner {
		v := siteapi.SiteWorkspacesAccountLinkForbidden(problem(http.StatusForbidden, "only the owner can open the account link"))
		return &v, nil
	}
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

// SiteWorkspacesSetSecondFactorRequirement switches the Workspace's Two-factor
// requirement on or off; owner and admin only. Turning it off is the one operation
// a User withheld by the requirement may still perform in the Workspace, so an
// Owner or Admin without a Second factor is never locked out of lifting it.
func (h *Handlers) SiteWorkspacesSetSecondFactorRequirement(ctx context.Context, req *siteapi.SiteSecondFactorRequirementInput, params siteapi.SiteWorkspacesSetSecondFactorRequirementParams) (siteapi.SiteWorkspacesSetSecondFactorRequirementRes, error) {
	load := h.membershipFor
	if !req.Required {
		load = h.membershipEvenIfWithheld
	}
	s, m, err := load(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesSetSecondFactorRequirementNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageSecondFactor(m.Role) {
		v := siteapi.SiteWorkspacesSetSecondFactorRequirementForbidden(problem(http.StatusForbidden, "only owners and admins can change the two-factor requirement"))
		return &v, nil
	}
	w, switchedOn, err := h.accounts.SetSecondFactorRequirement(ctx, s, h.actor(ctx), req.Required, h.now())
	if err != nil {
		return nil, err
	}
	// The requirement is committed; the email is best-effort, like the invite.
	if switchedOn {
		if merr := h.sysmail.EnqueueSecondFactorRequired(ctx, s.WorkspaceID()); merr != nil {
			slog.WarnContext(ctx, "second factor required email not enqueued", "error", merr, "workspace_id", s.WorkspaceID())
		}
	}
	r, err := h.workspaceResource(ctx, s, withWorkspace(m, w))
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SiteWorkspacesUpdate renames a workspace owned by the authenticated user. The
// slug is immutable, so only the display name changes.
func (h *Handlers) SiteWorkspacesUpdate(ctx context.Context, req *siteapi.SiteUpdateWorkspaceInput, params siteapi.SiteWorkspacesUpdateParams) (siteapi.SiteWorkspacesUpdateRes, error) {
	s, m, err := h.membershipFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		v := siteapi.SiteWorkspacesUpdateUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.name_empty", nil),
			map[string][]string{"name": {i18n.T("errors.name_empty", nil)}},
		))
		return &v, nil
	}

	var postal *string
	// postalAddress is optional in the contract: absent = leave unchanged, present
	// (incl. empty string) = set/clear. Trimmed so a whitespace-only value clears it.
	if req.PostalAddress.Set {
		v := strings.TrimSpace(req.PostalAddress.Value)
		postal = &v
	}
	w, err := h.accounts.UpdateWorkspace(ctx, s, h.actor(ctx), name, postal)
	if err != nil {
		return nil, err
	}
	resource, err := h.workspaceResource(ctx, s, withWorkspace(m, w))
	if err != nil {
		return nil, err
	}
	return &resource, nil
}
