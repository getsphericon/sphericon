package site

import (
	"context"
	"encoding/json"

	"github.com/go-faster/jx"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/mailjobs"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// SitePasskeysList lists the session User's Passkeys, oldest first (ADR 0052).
func (h *Handlers) SitePasskeysList(ctx context.Context) (*siteapi.SitePasskeyList, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := h.passkeys.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := mapper.PasskeysToResources(rows)
	return &siteapi.SitePasskeyList{
		Items: items, Available: h.passkeys.Available(), PasswordSet: u.PasswordHash != "", IdentityProofFresh: u.PasswordHash == "" && h.identityProofFresh(ctx),
	}, nil
}

// SitePasskeysBeginRegistration answers the creation options for the browser and sets the
// ceremony cookie. It writes nothing.
func (h *Handlers) SitePasskeysBeginRegistration(ctx context.Context) (siteapi.SitePasskeysBeginRegistrationRes, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	raw, ceremony, err := h.passkeys.BeginRegistration(ctx, u)
	if err != nil {
		return nil, err
	}
	options, err := decodeOptions[siteapi.SitePasskeyCreationOptions](raw)
	if err != nil {
		return nil, err
	}
	return &siteapi.SitePasskeyRegistrationBegunHeaders{
		SetCookie: h.ceremonyCookie(auth.PasskeyRegistrationPath, ceremony),
		Response:  siteapi.SitePasskeyRegistrationBegun{Options: options},
	}, nil
}

// SitePasskeysFinishRegistration stores the Passkey once the User proves their identity (the
// password, or a fresh provider login) and the authenticator's response checks out, and mails
// the owner. Every session of the User goes on.
func (h *Handlers) SitePasskeysFinishRegistration(ctx context.Context, req *siteapi.SitePasskeyRegistrationInput, params siteapi.SitePasskeysFinishRegistrationParams) (siteapi.SitePasskeysFinishRegistrationRes, error) {
	name, err := secondfactor.PasskeyName(string(req.Name))
	if err != nil {
		return nil, err
	}
	userID, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	response, err := credentialJSON(req.Credential)
	if err != nil {
		return nil, err
	}
	u, row, err := h.passkeys.FinishRegistration(ctx, userID, params.PASSKEY.Or(""), response, name)
	if err != nil {
		return nil, err
	}
	if err := h.mail.Enqueue(ctx, mailjobs.PasskeyNotice(u.Email, mailjobs.PasskeyAdded, row.Name)); err != nil {
		mailNotQueued(ctx, "PasskeyNotice", err)
	}
	res := mapper.PasskeyToResource(row)
	return &res, nil
}

// SitePasskeysBeginCheck answers the request options of a fresh Passkey check of the session
// User, who holds at least one Passkey, and sets the ceremony cookie.
func (h *Handlers) SitePasskeysBeginCheck(ctx context.Context) (siteapi.SitePasskeysBeginCheckRes, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	raw, ceremony, err := h.passkeys.BeginCheck(ctx, u)
	if err != nil {
		return nil, err
	}
	options, err := decodeOptions[siteapi.SitePasskeyRequestOptions](raw)
	if err != nil {
		return nil, err
	}
	return &siteapi.SitePasskeyCheckBegunHeaders{
		SetCookie: h.ceremonyCookie(auth.PasskeyCheckPath, ceremony),
		Response:  siteapi.SitePasskeyCheckBegun{Options: options},
	}, nil
}

// SitePasskeysFinishCheck verifies a fresh Passkey check of the session's own User (the
// passkeys module's Check, whose every refusal is the one 403). The acting session is reissued
// with the time of the check, which stands in for the password for a few minutes
// (identityProofTTL).
func (h *Handlers) SitePasskeysFinishCheck(ctx context.Context, req *siteapi.SitePasskeyCheckInput, params siteapi.SitePasskeysFinishCheckParams) (siteapi.SitePasskeysFinishCheckRes, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	response, err := credentialJSON(req.Credential)
	if err != nil {
		return nil, err
	}
	if err := h.passkeys.Check(ctx, u, params.PASSKEY.Or(""), response); err != nil {
		return nil, err
	}
	sess := currentSession(ctx)
	sess.PasskeyCheckAt = h.now()
	cookie, err := h.sessions.Issue(u, sess)
	if err != nil {
		return nil, err
	}
	return &siteapi.SitePasskeysFinishCheckNoContent{SetCookie: cookie.String()}, nil
}

// credentialJSON is the authenticator's response as the JSON the ceremony verifies. The
// contract's credential is a map of raw members, which json.Marshal would encode as base64.
func credentialJSON[M ~map[string]jx.Raw](credential M) ([]byte, error) {
	members := make(map[string]json.RawMessage, len(credential))
	for k, v := range credential {
		members[k] = json.RawMessage(v)
	}
	return json.Marshal(members)
}

// decodeOptions reads the browser options a ceremony's begin step produced into the
// contract's type.
func decodeOptions[T any](raw []byte) (T, error) {
	var options T
	err := json.Unmarshal(raw, &options)
	return options, err
}

// ceremonyCookie is the Set-Cookie value that carries a begun ceremony to its finish step.
func (h *Handlers) ceremonyCookie(path, ceremony string) string {
	return h.sessions.PasskeyCeremony(path, ceremony, secondfactor.CeremonyTTL).String()
}

// SitePasskeysRename renames a Passkey of the session User.
func (h *Handlers) SitePasskeysRename(ctx context.Context, req *siteapi.SitePasskeyRenameInput, params siteapi.SitePasskeysRenameParams) (siteapi.SitePasskeysRenameRes, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	row, err := h.passkeys.Rename(ctx, userID, int64(params.ID), string(req.Name))
	if err != nil {
		return nil, err
	}
	res := mapper.PasskeyToResource(row)
	return &res, nil
}

// SitePasskeysRemove removes a Passkey of the session User once they prove their identity (the
// password, or a fresh provider login). It ends the other sessions, reissues the acting one and
// mails the owner.
func (h *Handlers) SitePasskeysRemove(ctx context.Context, req *siteapi.SitePasskeyRemoveInput, params siteapi.SitePasskeysRemoveParams) (siteapi.SitePasskeysRemoveRes, error) {
	userID, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	u, row, err := h.passkeys.Remove(ctx, userID, int64(params.ID))
	if err != nil {
		return nil, err
	}
	remaining, err := h.passkeys.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	sess := currentSession(ctx)
	if len(remaining) == 0 {
		sess = sess.WithoutPasskeys()
	}
	cookie, err := h.sessions.Issue(u, sess)
	if err != nil {
		return nil, err
	}
	if err := h.mail.Enqueue(ctx, mailjobs.PasskeyNotice(u.Email, mailjobs.PasskeyRemoved, row.Name)); err != nil {
		mailNotQueued(ctx, "PasskeyNotice", err)
	}
	return &siteapi.SitePasskeysRemoveNoContent{SetCookie: cookie.String()}, nil
}
