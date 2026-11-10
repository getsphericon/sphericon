package site

import (
	"context"
	"strings"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

// SiteInvitationsList returns the workspace's pending (unaccepted) invitations.
func (h *Handlers) SiteInvitationsList(ctx context.Context, _ siteapi.SiteInvitationsListParams) ([]siteapi.SiteInvitationResource, error) {
	s := workspaceScope(ctx)

	invites, err := invitations.PendingInvitations(ctx, s)
	if err != nil {
		return nil, err
	}

	items := mapper.InvitationsToResources(invites)
	return items, nil
}

// SiteInvitationsCreate invites an email address to the workspace. Owner/admin
// only. The one-time accept link is returned (copy-link path), and the Invitation's
// event mails it to the invitee through the System sender when the Instance has one.
func (h *Handlers) SiteInvitationsCreate(ctx context.Context, req *siteapi.SiteCreateInvitationInput, _ siteapi.SiteInvitationsCreateParams) (*siteapi.SiteCreateInvitationResponse, error) {
	s := workspaceScope(ctx)
	a := auth.GetSiteAuth(ctx)
	in := inputs.InvitationCreate(req)
	in.InvitedBy = a.User.ID
	// Token, hash, expiry and the upsert are the accounts operation's; the site only
	// maps its refusals.
	inv, token, err := h.invitations.Invite(ctx, s, h.actor(ctx), in)
	if err != nil {
		return nil, err
	}

	// The copy-link is the invite's own delivery, so an Instance without outbound mail still
	// invites; the Invitation's event mails the link too.
	inviteURL := strings.TrimRight(h.appURL, "/") + "/invitations/" + token

	inv.Edges.Inviter = a.User
	return &siteapi.SiteCreateInvitationResponse{
		InviteUrl: inviteURL,
		Resource:  mapper.InvitationToResource(inv),
	}, nil
}

// SiteInvitationsDelete revokes a pending invitation. Owner/admin only.
func (h *Handlers) SiteInvitationsDelete(ctx context.Context, params siteapi.SiteInvitationsDeleteParams) error {
	s := workspaceScope(ctx)

	id := int64(params.ID)

	return h.invitations.RevokeInvitation(ctx, s, h.actor(ctx), id)
}

// SitePublicInvitationsLookup reveals a pending invite (workspace + email) so the
// public accept page can render. Session-less: the token is the authorization.
func (h *Handlers) SitePublicInvitationsLookup(ctx context.Context, params siteapi.SitePublicInvitationsLookupParams) (*siteapi.SiteInvitationLookupResult, error) {
	inv, err := h.invitations.PendingInvitation(ctx, params.Token)
	if err != nil {
		return nil, err
	}

	return mapper.InvitationToLookupResult(inv), nil
}

// SitePublicInvitationsAccept turns an invite into a Membership: it attaches an
// Verified User (their own session) or sets up the account (name + password and the
// answers to the Sign-up items, ADR 0062), then joins them to the workspace. Link possession authorizes an address with no Verified
// User; a Verified User's address also needs that User's session.
func (h *Handlers) SitePublicInvitationsAccept(ctx context.Context, req *siteapi.SiteAcceptInvitationInput, params siteapi.SitePublicInvitationsAcceptParams) error {
	// A Verified User's own session is the proof of the address (ADR 0030); the
	// transaction decides whether the address is one, so the session is always passed.
	sessionEmail := ""
	if a := auth.GetSiteAuth(ctx); a != nil {
		sessionEmail = a.User.Email
	}
	name, _ := req.Name.Get()
	password, _ := req.Password.Get()
	// Whether a name and a password are needed at all, and their rules, are the acceptance's to
	// say (a Verified User's address needs neither).
	return h.users.AcceptInvitationLink(ctx, params.Token, users.InvitationAcceptance{
		Name: string(name), Password: password, SignupItems: inputs.SignupAnswers(req.SignupItems), SessionEmail: sessionEmail,
	})
}
