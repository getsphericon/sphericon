package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent/membership"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
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

// SiteMembershipsUpdate changes a member's role (members:manage); granting the owner role or
// changing an owner's needs owners:manage (refused 403 permission_required), and the last owner
// can never be demoted.
func (h *Handlers) SiteMembershipsUpdate(ctx context.Context, req *siteapi.SiteUpdateMembershipInput, params siteapi.SiteMembershipsUpdateParams) (siteapi.SiteMembershipsUpdateRes, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)

	updated, err := h.accounts.ChangeMembershipRole(ctx, s, h.actor(ctx), callerMembership(ctx).Role, id, membership.Role(req.Role))
	if err != nil {
		return nil, err
	}
	resource := mapper.MembershipToResource(updated)
	return &resource, nil
}

// SiteMembershipsDelete removes a member (members:manage); removing an owner needs
// owners:manage, and the last owner cannot be removed.
func (h *Handlers) SiteMembershipsDelete(ctx context.Context, params siteapi.SiteMembershipsDeleteParams) (siteapi.SiteMembershipsDeleteRes, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)

	err := h.accounts.RemoveMembership(ctx, s, h.actor(ctx), callerMembership(ctx).Role, id)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteMembershipsDeleteNoContent{}, nil
}

// SiteMembershipsResetSecondFactor resets a member's Second factor (ADR 0020):
// second_factor:manage, and owners:manage for an owner's. It ends every session of the member
// and records `user.second_factor_reset` in this Workspace; the acting session is
// someone else's, so nothing is reissued. A member resets nobody, and nobody resets
// their own here (disabling takes a password and a code).
func (h *Handlers) SiteMembershipsResetSecondFactor(ctx context.Context, params siteapi.SiteMembershipsResetSecondFactorParams) (siteapi.SiteMembershipsResetSecondFactorRes, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)
	callerID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	target, err := accounts.SecondFactorResetTarget(ctx, s, callerID, callerMembership(ctx).Role, id)
	if err != nil {
		return nil, err
	}

	err = h.secondFactor.Reset(ctx, target.UserID, h.actor(ctx), s.WorkspaceID())
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteMembershipsResetSecondFactorNoContent{}, nil
}
