package site

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// notMeTTL is how long the "this was not me" link of an identity notice works.
const notMeTTL = 14 * 24 * time.Hour

// claimIdentity names the Linked identity a "this was not me" token undoes.
const claimIdentity = "identity"

// noticeIdentityLinked mails the owner that a Sign-in provider was linked to the account
// silently (ADR 0048), with the link that undoes it. Best-effort, like the other account
// notices: the sign-in it follows has already succeeded, and the visitor's response must not
// depend on the mail (it is the same for an address that existed and one that did not).
func (h *Handlers) noticeIdentityLinked(ctx context.Context, u *ent.User, provider string, identityID int64) {
	token, err := h.tokens.Mint(authtoken.PurposeIdentityNotMe, u.ID, "", notMeTTL,
		map[string]string{claimIdentity: strconv.FormatInt(identityID, 10)})
	if err != nil {
		slog.WarnContext(ctx, "signin: could not mint the not-me link", "error", err)
		return
	}
	if err := h.sysmail.EnqueueIdentityLinkedNotice(ctx, u.Email, h.providerDisplayName(provider), token); err != nil {
		slog.WarnContext(ctx, "signin: could not queue the linked-identity notice", "error", err)
	}
}

// providerDisplayName is what a provider is called to a person ("Google"); its registry name
// when the Instance no longer offers it.
func (h *Handlers) providerDisplayName(name string) string {
	for _, p := range h.signin.Enabled() {
		if p.Name == name {
			return p.DisplayName
		}
	}
	return name
}

// SiteAuthSigninNotMe is the link of the notice mailed after a silent link: it unlinks the
// identity the token names and ends every session of its User (ADR 0048). The token is the
// credential, so it works without a session; it works once, because once the identity is
// gone there is nothing left to undo.
func (h *Handlers) SiteAuthSigninNotMe(ctx context.Context, req *siteapi.SiteSigninNotMeInput) (siteapi.SiteAuthSigninNotMeRes, error) {
	uid, extra, err := h.tokens.Parse(req.Token, authtoken.PurposeIdentityNotMe, func(int64) (string, error) { return "", nil })
	identityID, parseErr := strconv.ParseInt(extra[claimIdentity], 10, 64)
	if err != nil || parseErr != nil {
		v := problem(http.StatusBadRequest, i18n.T("errors.not_me_link_invalid", nil))
		return &v, nil
	}
	removed, err := h.accounts.UndoLink(ctx, uid, identityID)
	if err != nil {
		return nil, err
	}
	if !removed {
		v := problem(http.StatusBadRequest, i18n.T("errors.not_me_link_invalid", nil))
		return &v, nil
	}
	return &siteapi.SiteAuthSigninNotMeOK{}, nil
}
