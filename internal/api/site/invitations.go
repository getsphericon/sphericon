package site

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/invitation"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// invitationResource projects an Invitation (optionally with its inviter edge)
// into the site DTO.
func invitationResource(inv *ent.Invitation) siteapi.SiteInvitationResource {
	invitedBy := siteapi.OptNilString{}
	if inv.Edges.Inviter != nil {
		invitedBy = siteapi.NewOptNilString(inv.Edges.Inviter.Email)
	}
	return siteapi.SiteInvitationResource{
		ID:             siteapi.EntityId(strconv.FormatInt(inv.ID, 10)),
		Email:          siteapi.EmailAddress(inv.Email),
		Role:           siteapi.SiteInvitableRole(inv.Role),
		ExpiresAt:      siteapi.Timestamp(inv.ExpiresAt),
		InvitedByEmail: invitedBy,
		CreatedAt:      siteapi.Timestamp(inv.CreatedAt),
	}
}

// SiteInvitationsList returns the workspace's pending (unaccepted) invitations.
func (h *Handlers) SiteInvitationsList(ctx context.Context, params siteapi.SiteInvitationsListParams) (siteapi.SiteInvitationsListRes, error) {
	s, _, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	invites, err := s.Invitation().Query().
		Where(invitation.AcceptedAtIsNil()).
		WithInviter().
		Order(ent.Asc(invitation.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	items := make(siteapi.SiteInvitationsListOKApplicationJSON, len(invites))
	for i, inv := range invites {
		items[i] = invitationResource(inv)
	}
	return &items, nil
}

// SiteInvitationsCreate invites an email address to the workspace. Owner/admin
// only. The one-time accept link is returned (copy-link path) and an invite email
// is sent best-effort — a missing/failed mailer must not fail the invite.
func (h *Handlers) SiteInvitationsCreate(ctx context.Context, req *siteapi.SiteCreateInvitationInput, params siteapi.SiteInvitationsCreateParams) (siteapi.SiteInvitationsCreateRes, error) {
	s, callerRole, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteInvitationsCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageMembers(callerRole) {
		v := siteapi.SiteInvitationsCreateForbidden(problem(http.StatusForbidden, "insufficient role"))
		return &v, nil
	}

	a := auth.GetSiteAuth(ctx)
	// Token, hash, expiry and the upsert are the accounts operation's; the site only
	// maps its refusals.
	inv, token, err := h.accounts.Invite(ctx, s, h.actor(ctx), accounts.InviteInput{
		Email: string(req.Email), Role: invitation.Role(req.Role), InvitedBy: a.UserID,
	})
	switch {
	case errors.Is(err, accounts.ErrEmailEmpty):
		v := siteapi.SiteInvitationsCreateUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity, i18n.T("errors.email_empty", nil),
			map[string][]string{"email": {i18n.T("errors.email_empty", nil)}}))
		return &v, nil
	case errors.Is(err, accounts.ErrAlreadyMember):
		v := siteapi.SiteInvitationsCreateConflict(problem(http.StatusConflict, i18n.T("errors.already_member", nil)))
		return &v, nil
	case err != nil:
		return nil, err
	}

	inviteURL := strings.TrimRight(h.appURL, "/") + "/invitations/" + token

	// Send the invite email best-effort: the copy-link above already delivered
	// the invite, so self-hosted instances without SMTP still work.
	wsEnt, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	inviterName := ""
	if caller, cerr := h.accounts.User(ctx, a.UserID); cerr == nil {
		inviterName = caller.Name
	}
	if merr := h.sysmail.EnqueueMemberInvite(ctx, inv.Email, inviteURL, wsEnt.Name, inviterName); merr != nil {
		slog.WarnContext(ctx, "member invite email not enqueued", "error", merr, "invitation_id", inv.ID)
	}

	inv.Edges.Inviter, _ = h.accounts.User(ctx, a.UserID)
	return &siteapi.SiteCreateInvitationResponse{
		InviteUrl: inviteURL,
		Resource:  invitationResource(inv),
	}, nil
}

// SiteInvitationsDelete revokes a pending invitation. Owner/admin only.
func (h *Handlers) SiteInvitationsDelete(ctx context.Context, params siteapi.SiteInvitationsDeleteParams) (siteapi.SiteInvitationsDeleteRes, error) {
	s, callerRole, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteInvitationsDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageMembers(callerRole) {
		v := siteapi.SiteInvitationsDeleteForbidden(problem(http.StatusForbidden, "insufficient role"))
		return &v, nil
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteInvitationsDeleteNotFound(problem(http.StatusNotFound, "invitation not found"))
		return &v, nil
	}

	found, err := h.accounts.RevokeInvitation(ctx, s, h.actor(ctx), id)
	if err != nil {
		return nil, err
	}
	if !found {
		v := siteapi.SiteInvitationsDeleteNotFound(problem(http.StatusNotFound, "invitation not found"))
		return &v, nil
	}
	return &siteapi.SiteInvitationsDeleteNoContent{}, nil
}

// validInvitation looks up a pending, unexpired invitation by its raw token.
func (h *Handlers) validInvitation(ctx context.Context, token string) (*ent.Invitation, error) {
	return h.accounts.PendingInvitation(ctx, token)
}

// SitePublicInvitationsLookup reveals a pending invite (workspace + email) so the
// public accept page can render. Session-less: the token is the authorization.
func (h *Handlers) SitePublicInvitationsLookup(ctx context.Context, params siteapi.SitePublicInvitationsLookupParams) (siteapi.SitePublicInvitationsLookupRes, error) {
	inv, err := h.validInvitation(ctx, params.Token)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "invitation not found or expired")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	hasAccount, err := h.accounts.HasVerifiedAccount(ctx, inv.Email)
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteInvitationLookupResult{
		WorkspaceName: inv.Edges.Workspace.Name,
		Email:         siteapi.EmailAddress(inv.Email),
		HasAccount:    hasAccount,
	}, nil
}

// SitePublicInvitationsAccept turns an invite into a Membership: it attaches an
// existing User or creates one (name + password), then joins them to the
// workspace. Link possession is the authorization.
func (h *Handlers) SitePublicInvitationsAccept(ctx context.Context, req *siteapi.SiteAcceptInvitationInput, params siteapi.SitePublicInvitationsAcceptParams) (siteapi.SitePublicInvitationsAcceptRes, error) {
	inv, err := h.validInvitation(ctx, params.Token)
	if ent.IsNotFound(err) {
		v := siteapi.SitePublicInvitationsAcceptNotFound(problem(http.StatusNotFound, "invitation not found or expired"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	name, _ := req.Name.Get()
	name = strings.TrimSpace(name)
	password, _ := req.Password.Get()

	err = h.accounts.AcceptInvitation(ctx, inv, name, password)
	if errors.Is(err, accounts.ErrCredentialsRequired) {
		// No Verified User for the address: the invitee sets up the account.
		v := siteapi.SitePublicInvitationsAcceptUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity, "name and password are required",
			map[string][]string{
				"name":     {"name is required"},
				"password": {"password is required"},
			}))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	return &siteapi.SitePublicInvitationsAcceptOK{}, nil
}
