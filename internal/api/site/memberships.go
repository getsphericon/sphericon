package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// SiteMembershipsList returns the workspace's members. Any member may view them.
func (h *Handlers) SiteMembershipsList(ctx context.Context, _ siteapi.SiteMembershipsListParams) (siteapi.SiteMembershipsListRes, error) {
	s := workspaceScope(ctx)

	members, err := accounts.Members(ctx, s)
	if err != nil {
		return nil, err
	}

	items := make(siteapi.SiteMembershipsListOKApplicationJSON, len(members))
	for i, m := range members {
		items[i] = mapper.MembershipToResource(m)
	}
	return &items, nil
}

// SiteMembershipsUpdate changes a member's role. Owner/admin only; only an owner
// may grant the owner role, and the last owner can never be demoted.
func (h *Handlers) SiteMembershipsUpdate(ctx context.Context, req *siteapi.SiteUpdateMembershipInput, params siteapi.SiteMembershipsUpdateParams) (siteapi.SiteMembershipsUpdateRes, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)

	updated, err := h.accounts.ChangeMembershipRole(ctx, s, h.actor(ctx), callerMembership(ctx).Role, id, membership.Role(req.Role))
	switch {
	case ent.IsNotFound(err):
		v := siteapi.SiteMembershipsUpdateNotFound(problem(http.StatusNotFound, "member not found"))
		return &v, nil
	case errors.Is(err, accounts.ErrOwnerOnly):
		v := siteapi.SiteMembershipsUpdateForbidden(problem(http.StatusForbidden, "only an owner may grant the owner role or change an owner's role"))
		return &v, nil
	case errors.Is(err, accounts.ErrLastOwner):
		v := siteapi.SiteMembershipsUpdateUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			"cannot demote the last owner",
			map[string][]string{"role": {i18n.T("errors.keep_one_owner", nil)}},
		))
		return &v, nil
	case err != nil:
		return nil, err
	}
	resource := mapper.MembershipToResource(updated)
	return &resource, nil
}

// SiteMembershipsDelete removes a member. Owner/admin only; the last owner cannot
// be removed.
func (h *Handlers) SiteMembershipsDelete(ctx context.Context, params siteapi.SiteMembershipsDeleteParams) (siteapi.SiteMembershipsDeleteRes, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)

	err := h.accounts.RemoveMembership(ctx, s, h.actor(ctx), callerMembership(ctx).Role, id)
	switch {
	case ent.IsNotFound(err):
		v := siteapi.SiteMembershipsDeleteNotFound(problem(http.StatusNotFound, "member not found"))
		return &v, nil
	case errors.Is(err, accounts.ErrOwnerOnly):
		v := siteapi.SiteMembershipsDeleteForbidden(problem(http.StatusForbidden, "only an owner may remove an owner"))
		return &v, nil
	case errors.Is(err, accounts.ErrLastOwner):
		v := siteapi.SiteMembershipsDeleteUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			"cannot remove the last owner",
			map[string][]string{"member": {i18n.T("errors.keep_one_owner", nil)}},
		))
		return &v, nil
	case err != nil:
		return nil, err
	}
	return &siteapi.SiteMembershipsDeleteNoContent{}, nil
}

// SiteMembershipsResetSecondFactor resets a member's Second factor (ADR 0020):
// owner/admin only, owner-only for an owner's. It ends every session of the member
// and records `user.second_factor_reset` in this Workspace; the acting session is
// someone else's, so nothing is reissued. A member resets nobody, and nobody resets
// their own here (disabling takes a password and a code).
func (h *Handlers) SiteMembershipsResetSecondFactor(ctx context.Context, params siteapi.SiteMembershipsResetSecondFactorParams) (siteapi.SiteMembershipsResetSecondFactorRes, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)
	target, err := accounts.SecondFactorResetTarget(ctx, s, callerMembership(ctx).Role, id)
	switch {
	case ent.IsNotFound(err):
		v := siteapi.SiteMembershipsResetSecondFactorNotFound(problem(http.StatusNotFound, "member not found"))
		return &v, nil
	case errors.Is(err, accounts.ErrOwnerOnly):
		v := siteapi.SiteMembershipsResetSecondFactorForbidden(problem(http.StatusForbidden, "only an owner may reset an owner's second factor"))
		return &v, nil
	case err != nil:
		return nil, err
	}
	if a := auth.GetSiteAuth(ctx); a != nil && a.UserID == target.UserID {
		v := siteapi.SiteMembershipsResetSecondFactorUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			"cannot reset your own second factor",
			map[string][]string{"member": {i18n.T("errors.second_factor_reset_self", nil)}},
		))
		return &v, nil
	}

	err = h.secondFactor.Reset(ctx, target.UserID, h.actor(ctx), s.WorkspaceID())
	if errors.Is(err, secondfactor.ErrNotActive) {
		v := siteapi.SiteMembershipsResetSecondFactorUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			"the member has no second factor or passkey",
			map[string][]string{"member": {i18n.T("errors.second_factor_not_active", nil)}},
		))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteMembershipsResetSecondFactorNoContent{}, nil
}
