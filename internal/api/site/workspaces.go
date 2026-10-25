package site

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accountlink"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/reportday"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
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
	r.McpUrl = h.mcpURL
	r.TrackerUrl = h.trackerURL
	r.AccountLinkAvailable = h.accountLink != nil && accounts.IsOwner(m.Role)
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
	name := strings.TrimSpace(req.Name)
	if name == "" {
		v := siteapi.SiteWorkspacesCreateUnprocessableEntity(nameEmptyProblem())
		return &v, nil
	}
	ws, err := h.accounts.CreateWorkspace(ctx, a.UserID, name)
	if errors.Is(err, accounts.ErrEmailNotVerified) {
		v := siteapi.SiteWorkspacesCreateForbidden(siteapi.ProblemDetails{
			Code:   siteapi.NewOptProblemCode(siteapi.ProblemCodeEmailVerificationRequired),
			Status: siteapi.NewOptInt32(http.StatusForbidden),
			Title:  siteapi.NewOptString(http.StatusText(http.StatusForbidden)),
			Detail: siteapi.NewOptString(i18n.T("errors.email_verification_required", nil)),
		})
		return &v, nil
	}
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

func nameEmptyProblem() siteapi.ProblemDetails {
	return problemWithErrors(
		http.StatusUnprocessableEntity,
		i18n.T("errors.name_empty", nil),
		map[string][]string{fieldName: {i18n.T("errors.name_empty", nil)}},
	)
}

// SiteWorkspacesAccountLink mints the Owner's Account link token. Without the
// installation's configuration it answers 404 before anything else; any role but
// Owner is refused.
func (h *Handlers) SiteWorkspacesAccountLink(ctx context.Context, params siteapi.SiteWorkspacesAccountLinkParams) (siteapi.SiteWorkspacesAccountLinkRes, error) {
	if h.accountLink == nil {
		v := siteapi.SiteWorkspacesAccountLinkNotFound(problem(http.StatusNotFound, "account link is not configured"))
		return &v, nil
	}
	s, m, err := h.membershipFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesAccountLinkNotFound(workspaceNotFoundProblem())
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.IsOwner(m.Role) {
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
	s, m, err := load(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesSetSecondFactorRequirementNotFound(workspaceNotFoundProblem())
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

// SiteWorkspacesUpdate renames a workspace (Owner and Admin only). The id never changes.
func (h *Handlers) SiteWorkspacesUpdate(ctx context.Context, req *siteapi.SiteUpdateWorkspaceInput, params siteapi.SiteWorkspacesUpdateParams) (siteapi.SiteWorkspacesUpdateRes, error) {
	s, m, err := h.membershipFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesUpdateNotFound(workspaceNotFoundProblem())
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	if !accounts.CanRenameWorkspace(m.Role) {
		v := siteapi.SiteWorkspacesUpdateForbidden(problem(http.StatusForbidden, "only owners and admins can rename the workspace"))
		return &v, nil
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		v := siteapi.SiteWorkspacesUpdateUnprocessableEntity(nameEmptyProblem())
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

// SiteWorkspacesSetTimezone sets the Workspace time zone (Owner only, ADR 0057).
func (h *Handlers) SiteWorkspacesSetTimezone(ctx context.Context, req *siteapi.SiteWorkspaceTimezoneInput, params siteapi.SiteWorkspacesSetTimezoneParams) (siteapi.SiteWorkspacesSetTimezoneRes, error) {
	s, m, err := h.membershipFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesSetTimezoneNotFound(workspaceNotFoundProblem())
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanSetTimezone(m.Role) {
		v := siteapi.SiteWorkspacesSetTimezoneForbidden(problem(http.StatusForbidden, "only owners can set the workspace time zone"))
		return &v, nil
	}
	w, err := h.accounts.SetTimezone(ctx, s, h.actor(ctx), string(req.Timezone))
	if errors.Is(err, reportday.ErrUnknownZone) {
		v := siteapi.SiteWorkspacesSetTimezoneUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.timezone_unknown", nil),
			map[string][]string{"timezone": {i18n.T("errors.timezone_unknown", nil)}},
		))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	resource, err := h.workspaceResource(ctx, s, withWorkspace(m, w))
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

func workspaceNotFoundProblem() siteapi.ProblemDetails {
	return problem(http.StatusNotFound, "workspace not found")
}

// deletedWorkspaceResource is a Workspace in its deletion window with the date its data
// will be erased.
func deletedWorkspaceResource(w *ent.Workspace, onlyOwner bool) siteapi.SiteDeletedWorkspaceResource {
	res := mapper.DeletedWorkspaceToResource(w)
	res.OnlyOwner = onlyOwner
	return res
}

// SiteWorkspacesListDeleted lists the caller's Owner Workspaces in their deletion window.
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
func (h *Handlers) SiteWorkspacesDelete(ctx context.Context, req *siteapi.SiteDeleteWorkspaceInput, params siteapi.SiteWorkspacesDeleteParams) (siteapi.SiteWorkspacesDeleteRes, error) {
	s, m, err := h.membershipFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesDeleteNotFound(workspaceNotFoundProblem())
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanDeleteWorkspace(m.Role) {
		v := siteapi.SiteWorkspacesDeleteForbidden(problem(http.StatusForbidden, "only owners can delete the workspace"))
		return &v, nil
	}
	w, err := h.deletion.Delete(ctx, s, h.actor(ctx), req.Name, h.now())
	if errors.Is(err, workspacedeletion.ErrNameMismatch) {
		v := siteapi.SiteWorkspacesDeleteUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.workspace_name_mismatch", nil),
			map[string][]string{fieldName: {i18n.T("errors.workspace_name_mismatch", nil)}},
		))
		return &v, nil
	}
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
func (h *Handlers) SiteWorkspacesLeave(ctx context.Context, params siteapi.SiteWorkspacesLeaveParams) (siteapi.SiteWorkspacesLeaveRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesLeaveNotFound(workspaceNotFoundProblem())
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	err = h.accounts.Leave(ctx, s, userID)
	if errors.Is(err, accounts.ErrLastOwner) {
		v := siteapi.SiteWorkspacesLeaveUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.leave_only_owner", nil),
			map[string][]string{"workspace": {i18n.T("errors.leave_only_owner", nil)}},
		))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteWorkspacesLeaveNoContent{}, nil
}

// SiteWorkspacesRestore restores a Workspace in its deletion window (Owner only).
func (h *Handlers) SiteWorkspacesRestore(ctx context.Context, params siteapi.SiteWorkspacesRestoreParams) (siteapi.SiteWorkspacesRestoreRes, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		v := siteapi.SiteWorkspacesRestoreNotFound(workspaceNotFoundProblem())
		return &v, nil
	}
	_, err := h.deletion.Restore(ctx, a.UserID, string(params.WorkspaceId), h.actor(ctx), h.now())
	if ent.IsNotFound(err) {
		v := siteapi.SiteWorkspacesRestoreNotFound(workspaceNotFoundProblem())
		return &v, nil
	}
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		v := siteapi.SiteWorkspacesRestoreConflict(problem(http.StatusConflict, i18n.T("errors.workspace_not_restorable", nil)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	// The Workspace is live again, so the caller's Owner Membership resolves like any other.
	s, m, err := h.membershipEvenIfWithheld(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	r, err := h.workspaceResource(ctx, s, m)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
