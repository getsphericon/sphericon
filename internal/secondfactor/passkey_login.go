package secondfactor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/passkey"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// ErrPasskeyUnknown means the response names no credential: it is malformed, or its credential id
// was never registered (or was removed).
var ErrPasskeyUnknown = errors.New("secondfactor: passkey credential unknown")

// BeginLogin starts a usernameless Passkey login: the request options for the browser (JSON)
// and the sealed ceremony cookie value. The User is not known yet, so the options allow no
// credential in particular and the browser offers the Passkeys it holds for this host. It
// writes nothing: the challenge's single use comes from the row FinishLogin inserts.
func (p *Passkeys) BeginLogin() (options []byte, ceremony string, err error) {
	if !p.Available() {
		return nil, "", ErrPasskeysUnavailable
	}
	assertion, session, err := p.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	if options, err = json.Marshal(assertion.Response); err != nil {
		return nil, "", err
	}
	if ceremony, err = p.seal(PurposeLogin, nil, session); err != nil {
		return nil, "", err
	}
	return options, ceremony, nil
}

// FindLogin returns the User a login response belongs to, by the credential id it carries.
// It verifies nothing: the caller consults the User's Login throttle and the verification
// gate first, then calls FinishLogin. ErrPasskeyUnknown when no credential matches.
func (p *Passkeys) FindLogin(ctx context.Context, response []byte) (*ent.User, error) {
	if !p.Available() {
		return nil, ErrPasskeysUnavailable
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return nil, ErrPasskeyUnknown
	}
	row, err := p.ent.Passkey.Query().Where(passkey.CredentialID(parsed.RawID)).WithUser().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrPasskeyUnknown
	}
	if err != nil {
		return nil, err
	}
	return row.Edges.User, nil
}

// FinishLogin verifies the authenticator's response (the browser's AuthenticationResponseJSON)
// against the ceremony cookie for User u, whom FindLogin named. The response must be signed
// for the cookie's challenge and this Instance's origin, by a credential u owns, with the user
// verified. One transaction spends the challenge, advances the signature counter and records
// the use.
//
// ErrCeremony for an unusable cookie, ErrChallengeSpent when the ceremony already finished,
// ErrPasskeyRefused for any response that does not verify (including one made without user
// verification, and one whose signature counter went backwards: a cloned authenticator).
func (p *Passkeys) FinishLogin(ctx context.Context, u *ent.User, ceremony string, response []byte) error {
	return p.finishAssertion(ctx, u, PurposeLogin, ceremony, response)
}

// BeginCheck starts a fresh Passkey check of the signed-in User u (ADR 0052): the request
// options for the browser (JSON), which allow only u's own Passkeys, and the sealed ceremony
// cookie value, bound to u and their session epoch. It writes nothing. ErrPasskeyUnknown when u
// holds no Passkey.
func (p *Passkeys) BeginCheck(ctx context.Context, u *ent.User) (options []byte, ceremony string, err error) {
	if !p.Available() {
		return nil, "", ErrPasskeysUnavailable
	}
	rows, err := p.List(ctx, u.ID)
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return nil, "", ErrPasskeyUnknown
	}
	assertion, session, err := p.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	for _, r := range rows {
		assertion.Response.AllowedCredentials = append(assertion.Response.AllowedCredentials,
			protocol.CredentialDescriptor{Type: protocol.PublicKeyCredentialType, CredentialID: r.CredentialID})
	}
	if options, err = json.Marshal(assertion.Response); err != nil {
		return nil, "", err
	}
	if ceremony, err = p.seal(PurposeCheck, u, session); err != nil {
		return nil, "", err
	}
	return options, ceremony, nil
}

// FinishCheck verifies a Passkey check for the signed-in User u, whom FindLogin named from the
// response: it is FinishLogin for a ceremony BeginCheck sealed, which must be u's and of u's
// current session epoch. It takes no part in the Login throttle: the caller is already signed
// in, and a refusal reveals nothing a guess could use. The errors are FinishLogin's.
func (p *Passkeys) FinishCheck(ctx context.Context, u *ent.User, ceremony string, response []byte) error {
	return p.finishAssertion(ctx, u, PurposeCheck, ceremony, response)
}

func (p *Passkeys) finishAssertion(ctx context.Context, u *ent.User, purpose ceremonyPurpose, ceremony string, response []byte) error {
	if !p.Available() {
		return ErrPasskeysUnavailable
	}
	claims, session, err := p.open(ceremony, purpose)
	if err != nil {
		return err
	}
	if purpose == PurposeCheck && (claims.UID != u.ID || claims.Epoch != u.SessionEpoch) {
		return ErrCeremony
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPasskeyRefused, err)
	}
	return p.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		rows, err := tx.Passkey.Query().Where(passkey.UserID(u.ID)).Order(passkey.ByID()).All(ctx)
		if err != nil {
			return err
		}
		handler := func(_, handle []byte) (webauthn.User, error) {
			owner, ok := UserIDOfHandle(handle)
			if !ok || owner != u.ID {
				return nil, ErrPasskeyUnknown
			}
			return waCredentials{waUser: waUser{u}, credentials: credentialsOf(rows)}, nil
		}
		_, cred, err := p.wa.ValidatePasskeyLogin(handler, *session, parsed)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrPasskeyRefused, err)
		}
		// The library checks the flag against the ceremony's requirement; the module does
		// not rely on that alone: a Passkey login proves the person (ADR 0052).
		if !parsed.Response.AuthenticatorData.Flags.HasUserVerified() || cred.Authenticator.CloneWarning {
			return ErrPasskeyRefused
		}
		if err := p.spend(ctx, tx, session.Challenge); err != nil {
			return err
		}
		row, ok := ownedBy(rows, cred.ID)
		if !ok {
			return ErrPasskeyRefused
		}
		return tx.Passkey.UpdateOneID(row.ID).
			SetSignCount(int64(cred.Authenticator.SignCount)).
			SetBackupState(cred.Flags.BackupState).
			SetLastUsedAt(p.now()).
			Exec(ctx)
	})
}

func ownedBy(rows []*ent.Passkey, credentialID []byte) (*ent.Passkey, bool) {
	for _, r := range rows {
		if bytes.Equal(r.CredentialID, credentialID) {
			return r, true
		}
	}
	return nil, false
}

// credentialsOf rebuilds the library's credentials from the stored rows. Every Passkey was
// registered with the user verified, so the latched flag is set.
func credentialsOf(rows []*ent.Passkey) []webauthn.Credential {
	out := make([]webauthn.Credential, len(rows))
	for i, r := range rows {
		transports := make([]protocol.AuthenticatorTransport, len(r.Transports))
		for j, t := range r.Transports {
			transports[j] = protocol.AuthenticatorTransport(t)
		}
		out[i] = webauthn.Credential{
			ID:              r.CredentialID,
			PublicKey:       r.PublicKey,
			AttestationType: r.AttestationType,
			Transport:       transports,
			Flags: webauthn.CredentialFlags{
				UserPresent:    true,
				UserVerified:   true,
				BackupEligible: r.BackupEligible,
				BackupState:    r.BackupState,
			},
			Authenticator: webauthn.Authenticator{AAGUID: r.Aaguid, SignCount: narrow.Uint32(r.SignCount)},
		}
	}
	return out
}

// waCredentials is a User as the library sees them at login: with the credentials they hold.
type waCredentials struct {
	waUser

	credentials []webauthn.Credential
}

func (w waCredentials) WebAuthnCredentials() []webauthn.Credential { return w.credentials }
