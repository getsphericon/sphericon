package site

import (
	"context"
	"log/slog"
	"strings"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
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

// inviteForm is what SiteInvitationsCreate checks beyond the contract, the address trimmed.
type inviteForm struct {
	Email string `json:"email" validate:"omitempty,account_email"`
}

// SiteInvitationsCreate invites an email address to the workspace. Owner/admin
// only. The one-time accept link is returned (copy-link path), and the Invitation's
// event mails it to the invitee through the System sender when the Instance has one.
func (h *Handlers) SiteInvitationsCreate(ctx context.Context, req *siteapi.SiteCreateInvitationInput, _ siteapi.SiteInvitationsCreateParams) (*siteapi.SiteCreateInvitationResponse, error) {
	s := workspaceScope(ctx)

	// An empty address is the accounts operation's refusal (ErrEmailEmpty); one that is sent
	// must be an address the invitee can receive mail at.
	if err := validation.Form(inviteForm{Email: strings.TrimSpace(string(req.Email))}); err != nil {
		return nil, err
	}

	a := auth.GetSiteAuth(ctx)
	in := inputs.InvitationCreate(req)
	in.InvitedBy = a.UserID
	// Token, hash, expiry and the upsert are the accounts operation's; the site only
	// maps its refusals.
	inv, token, err := h.invitations.Invite(ctx, s, h.actor(ctx), in)
	if err != nil {
		return nil, err
	}

	// The copy-link is the invite's own delivery, so an Instance without outbound mail still
	// invites; the Invitation's event mails the link too.
	inviteURL := strings.TrimRight(h.appURL, "/") + "/invitations/" + token

	inviter, uerr := h.users.User(ctx, a.UserID)
	if uerr != nil {
		slog.WarnContext(ctx, "inviter not loaded for the invitation", "error", uerr, "invitation_id", inv.ID)
	}
	inv.Edges.Inviter = inviter
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

// acceptForm is what SitePublicInvitationsAccept checks beyond the contract.
type acceptForm struct {
	Password string `json:"password" validate:"omitempty,password_not_blank,password_length"`
}

// SitePublicInvitationsAccept turns an invite into a Membership: it attaches an
// Verified User (their own session) or sets up the account (name + password and the
// answers to the Sign-up items, ADR 0062), then joins them to the workspace. Link possession authorizes an address with no Verified
// User; a Verified User's address also needs that User's session.
func (h *Handlers) SitePublicInvitationsAccept(ctx context.Context, req *siteapi.SiteAcceptInvitationInput, params siteapi.SitePublicInvitationsAcceptParams) error {
	inv, err := h.invitations.PendingInvitation(ctx, params.Token)
	if err != nil {
		return err
	}

	// A Verified User's own session is the proof of the address (ADR 0030); the
	// transaction decides whether the address is one, so the session is always passed.
	sessionEmail := ""
	if a := auth.GetSiteAuth(ctx); a != nil {
		sessionEmail = a.Email
	}
	name, _ := req.Name.Get()
	password, _ := req.Password.Get()
	// Whether a name and a password are needed at all is the transaction's to say (a Verified
	// User's address needs neither, and the name's rule is the transaction's too); a password
	// that is sent must pass its rule.
	if err := validation.Form(acceptForm{Password: password}); err != nil {
		return err
	}

	err = h.users.AcceptInvitation(ctx, inv, users.InvitationAcceptance{
		Name: strings.TrimSpace(string(name)), Password: password, SignupItems: inputs.SignupAnswers(req.SignupItems), SessionEmail: sessionEmail,
	})
	if err != nil {
		return err
	}

	return nil
}
