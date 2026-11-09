package site

import (
	"context"
	"encoding/json"

	"github.com/go-faster/jx"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
)

// SitePasskeysList lists the session User's Passkeys, oldest first (ADR 0052).
func (h *Handlers) SitePasskeysList(ctx context.Context) (*siteapi.SitePasskeyList, error) {
	u, err := sessionUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.passkeys.List(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	items := mapper.PasskeysToResources(rows)
	return &siteapi.SitePasskeyList{
		Items: items, Available: h.passkeys.Available(), PasswordSet: u.PasswordHash != "", IdentityProofFresh: sessionProof(ctx, "").StandsInFor(u, h.now()),
	}, nil
}

// SitePasskeysBeginRegistration answers the creation options for the browser and sets the
// ceremony cookie. It writes nothing.
func (h *Handlers) SitePasskeysBeginRegistration(ctx context.Context) (*siteapi.SitePasskeyRegistrationBegunHeaders, error) {
	u, err := sessionUser(ctx)
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
// password, or a fresh provider login) and the authenticator's response checks out; the owner
// is told by mail. Every session of the User goes on.
func (h *Handlers) SitePasskeysFinishRegistration(ctx context.Context, req *siteapi.SitePasskeyRegistrationInput, params siteapi.SitePasskeysFinishRegistrationParams) (*siteapi.SitePasskey, error) {
	u, proof, err := h.identityProof(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	response, err := credentialJSON(req.Credential)
	if err != nil {
		return nil, err
	}
	_, row, err := h.passkeys.FinishRegistration(ctx, u, proof, passkeys.Registration{Ceremony: params.PASSKEY.Or(""), Response: response, Name: string(req.Name)})
	if err != nil {
		return nil, err
	}
	res := mapper.PasskeyToResource(row)
	return &res, nil
}

// SitePasskeysBeginCheck answers the request options of a fresh Passkey check of the session
// User, who holds at least one Passkey, and sets the ceremony cookie.
func (h *Handlers) SitePasskeysBeginCheck(ctx context.Context) (*siteapi.SitePasskeyCheckBegunHeaders, error) {
	u, err := sessionUser(ctx)
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
// (users.IdentityProof.FreshAt).
func (h *Handlers) SitePasskeysFinishCheck(ctx context.Context, req *siteapi.SitePasskeyCheckInput, params siteapi.SitePasskeysFinishCheckParams) (*siteapi.SitePasskeysFinishCheckNoContent, error) {
	u, err := sessionUser(ctx)
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
	return h.sessions.PasskeyCeremony(path, ceremony, passkeys.CeremonyTTL).String()
}

// SitePasskeysRename renames a Passkey of the session User.
func (h *Handlers) SitePasskeysRename(ctx context.Context, req *siteapi.SitePasskeyRenameInput, params siteapi.SitePasskeysRenameParams) (*siteapi.SitePasskey, error) {
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
func (h *Handlers) SitePasskeysRemove(ctx context.Context, req *siteapi.SitePasskeyRemoveInput, params siteapi.SitePasskeysRemoveParams) (*siteapi.SitePasskeysRemoveNoContent, error) {
	u, proof, err := h.identityProof(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	removal, err := h.passkeys.Remove(ctx, u, proof, int64(params.ID))
	if err != nil {
		return nil, err
	}
	sess := currentSession(ctx)
	if removal.NoneLeft {
		sess = sess.WithoutPasskeys()
	}
	cookie, err := h.sessions.Issue(removal.User, sess)
	if err != nil {
		return nil, err
	}
	return &siteapi.SitePasskeysRemoveNoContent{SetCookie: cookie.String()}, nil
}
