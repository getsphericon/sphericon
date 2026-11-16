// Package passkeys owns a User's Passkeys (ADR 0052) and the spent challenges of their
// ceremonies: registering one, the usernameless login, the fresh check of a signed-in User,
// renaming and removing one. A Passkey belongs to the User, not a Workspace, so the package works
// on the raw *ent.Client. The User's own rows (the session epoch a removal bumps) are written
// through internal/domain/identity/users; clearing every Passkey of a User is part of a Second factor
// reset, which internal/domain/identity/secondfactors runs (Module.RemoveAllIn).
package passkeys

import (
	"context"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/passkey"
	"github.com/getsphericon/sphericon/ent/passkeychallenge"
	"github.com/getsphericon/sphericon/ent/useridentity"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// CeremonyTTL is how long a Passkey ceremony cookie lives: the time to touch the authenticator.
const CeremonyTTL = 5 * time.Minute

// SpentChallengeRetention is how long a spent challenge is kept before the daily cleanup
// prunes it. It is longer than CeremonyTTL, so a challenge cannot outlive its row: a replay
// of a pruned challenge fails on the cookie's expiry instead.
const SpentChallengeRetention = 24 * time.Hour

var (
	// ErrCeremony: the ceremony cookie is missing, forged, expired, another ceremony's or
	// another User's, or the User's sessions ended since it was minted.
	ErrCeremony = errors.New("passkeys: passkey ceremony cookie unusable")
	// ErrChallengeSpent: the ceremony already finished once.
	ErrChallengeSpent = errors.New("passkeys: passkey challenge already spent")
	// ErrPasskeysUnavailable: the Instance's public URL has no host name a Passkey can be bound
	// to (an IP address), so no Passkey can be registered or used on it.
	ErrPasskeysUnavailable = validation.NotFound("passkeys: passkeys need a host name, not an IP address", i18n.ErrorsPasskeysUnavailable)
	// ErrPasskeyRefused: the authenticator's response is not acceptable: malformed, not
	// signed for this challenge or origin, made without user verification, or for a
	// credential that is already registered.
	ErrPasskeyRefused = errors.New("passkeys: passkey response refused")
	// ErrCredentialRegistered: the authenticator's credential is already registered, to this User
	// or another one. It is an ErrPasskeyRefused.
	ErrCredentialRegistered = fmt.Errorf("%w: credential already registered", ErrPasskeyRefused)
	// ErrPasskeyNotFound: no Passkey of that id belongs to the User (another User's counts as
	// none).
	ErrPasskeyNotFound = validation.NotFound("passkeys: passkey not found", i18n.ErrorsPasskeyNotFound)

	// ErrRegistrationFailed: a registration ceremony that cannot finish (CeremonyFailed); the
	// User starts it again.
	ErrRegistrationFailed = validation.Refuse("passkeys: passkey registration failed", i18n.ErrorsPasskeyRegistrationFailed)
	// ErrLastSignIn: the Passkey is the last way the User can sign in: no password, no Linked
	// identity and no other Passkey.
	ErrLastSignIn = validation.Conflict("passkeys: the passkey is the last way to sign in", i18n.ErrorsPasskeyLastSignIn)
)

// constraints are the unique constraints of the Passkeys and their spent challenges (ADR 0106):
// a credential is registered once, and a challenge is spent once.
var constraints = db.Declare(
	db.Refusal("passkeys_credential_id_key", ErrCredentialRegistered),
	db.Refusal("passkey_challenges_challenge_key", ErrChallengeSpent),
)

type ceremonyPurpose string

const (
	// PurposeRegistration is the ceremony that adds a Passkey to a signed-in User.
	PurposeRegistration ceremonyPurpose = "register"
	// PurposeLogin is the usernameless login ceremony (the User is not known at `begin`).
	PurposeLogin ceremonyPurpose = "login"
	// PurposeCheck is the step-up ceremony of a signed-in User: a fresh Passkey check that
	// stands in for the password (ADR 0052). It is bound to the User and their session epoch.
	PurposeCheck ceremonyPurpose = "check"
)

// Module registers and checks a User's Passkeys (ADR 0052). The credentials are
// discoverable and user verification is required; attestation is not requested. The
// relying party id is the host of the Instance's public URL. A Passkey belongs to the
// User, so it works on the raw *ent.Client.
type Module struct {
	ent   *ent.Client
	bus   *events.Bus
	users Users
	party RelyingParty
	now   func() time.Time
}

// RelyingParty is who the Instance's Passkeys are bound to: the host of its public URL, the origin
// a ceremony may come from, and the key its ceremony cookie is sealed with. The zero value is the
// relying party of an Instance served on an IP address, on which no Passkey works.
type RelyingParty struct {
	wa  *webauthn.WebAuthn
	key []byte
}

// Users is the proof of identity a User's change of their own Passkeys takes
// (users.Module.ProveIdentity).
type Users interface {
	ProveIdentity(ctx context.Context, u *ent.User, proof users.IdentityProof) error
}

// New builds the module over the Instance's relying party (NewRelyingParty). identity proves who a
// User is who registers or removes a Passkey. now is the Instance clock the cookie's expiry is
// checked against and a Passkey's use is dated by.
func New(client *ent.Client, bus *events.Bus, identity Users, party RelyingParty, now func() time.Time) *Module {
	return &Module{ent: client, bus: bus, users: identity, party: party, now: now}
}

// NewRelyingParty is the relying party of the Instance at appURL (SPHERICON_APP_URL): its host
// is the relying party id and its origin the only one a ceremony may come from. An Instance
// served on an IP address gets the zero relying party, on which the module is not Available (a
// relying party id must be a domain), so booting never fails over it. secret is the Instance's JWT
// secret, from which the ceremony cookie's key is derived. The wiring builds it and hands it to New.
func NewRelyingParty(appURL, secret string) (RelyingParty, error) {
	u, err := url.Parse(appURL)
	if err != nil || u.Hostname() == "" {
		return RelyingParty{}, fmt.Errorf("passkeys: the app URL %q has no host", appURL)
	}
	if net.ParseIP(u.Hostname()) != nil {
		return RelyingParty{}, nil
	}
	required := true
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                  u.Hostname(),
		RPDisplayName:         users.Issuer,
		RPOrigins:             []string{u.Scheme + "://" + u.Host},
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			RequireResidentKey: &required,
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		},
		// The cookie's own expiry bounds a ceremony on the module's clock; the library's
		// check would read the wall clock.
		Timeouts: webauthn.TimeoutsConfig{
			Registration: webauthn.TimeoutConfig{Timeout: CeremonyTTL, TimeoutUVD: CeremonyTTL},
			Login:        webauthn.TimeoutConfig{Timeout: CeremonyTTL, TimeoutUVD: CeremonyTTL},
		},
	})
	if err != nil {
		return RelyingParty{}, fmt.Errorf("passkeys: %w", err)
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, "passkey_ceremony", sha256.Size)
	if err != nil {
		return RelyingParty{}, fmt.Errorf("passkeys: %w", err)
	}
	return RelyingParty{wa: wa, key: key}, nil
}

// Available reports whether Passkeys work on this Instance: its public URL has a host name.
func (p *Module) Available() bool { return p.party.wa != nil }

// UserHandle is the WebAuthn user handle of a User: the same for all their Passkeys, as the
// specification asks, and derived from the id so no column holds it. It carries no personal
// data. Changing it would orphan every registered Passkey.
func UserHandle(userID int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(max(userID, 0)))
}

// UserIDOfHandle is the User a handle names; false for a handle UserHandle never produced.
func UserIDOfHandle(handle []byte) (int64, bool) {
	if len(handle) != 8 {
		return 0, false
	}
	id := binary.BigEndian.Uint64(handle)
	if id > math.MaxInt64 {
		return 0, false
	}
	return int64(id), true
}

// CeremonyFailed reports whether err is a ceremony that cannot finish: its cookie is missing,
// expired or spent, or the authenticator's response does not verify.
func CeremonyFailed(err error) bool {
	return errors.Is(err, ErrCeremony) || errors.Is(err, ErrChallengeSpent) || errors.Is(err, ErrPasskeyRefused)
}

// List returns the User's Passkeys, oldest first.
func (p *Module) List(ctx context.Context, userID int64) ([]*ent.Passkey, error) {
	return p.ent.Passkey.Query().Where(passkey.UserID(userID)).Order(passkey.ByID()).All(ctx)
}

// RenameInput names the Passkey to rename and the name it is given.
type RenameInput struct {
	ID   int64
	Name string
}

// Rename gives the User's Passkey in.ID the name in.Name, held to the name rule (validation.Name);
// ErrPasskeyNotFound when it is not theirs.
func (p *Module) Rename(ctx context.Context, userID int64, in RenameInput) (*ent.Passkey, error) {
	name, err := validation.Name(in.Name)
	if err != nil {
		return nil, err
	}
	row, err := p.ent.Passkey.Query().Where(passkey.ID(in.ID), passkey.UserID(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrPasskeyNotFound
	}
	if err != nil {
		return nil, err
	}
	return p.ent.Passkey.UpdateOneID(row.ID).SetName(name).Save(ctx)
}

// Removal is what removing a Passkey left: the saved User, for reissuing the acting session, the
// removed Passkey, and whether the User holds no Passkey any more.
type Removal struct {
	User     *ent.User
	Passkey  *ent.Passkey
	NoneLeft bool
}

// RemoveInput names the Passkey to remove and carries the proof of identity its removal takes.
type RemoveInput struct {
	ID    int64
	Proof users.IdentityProof
}

// Remove deletes the Passkey in.ID of the User u once they prove their identity with in.Proof
// (users.Module.ProveIdentity), and bumps the session epoch, ending every session of the User, in
// one transaction, which publishes events.PasskeyChange (the owner is told, ADR 0105).
// ErrPasskeyNotFound when the Passkey is not theirs; ErrLastSignIn when it is the last way the
// User can sign in.
func (p *Module) Remove(ctx context.Context, u *ent.User, in RemoveInput) (Removal, error) {
	if err := p.users.ProveIdentity(ctx, u, in.Proof); err != nil {
		return Removal{}, err
	}
	userID, id := u.ID, in.ID
	var removal Removal
	err := p.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		// The User row is locked first, the order of every change of the User's Second factor and
		// Passkeys (a Second factor reset bumps the User, then removes the Passkeys): a removal and
		// a reset queue on it rather than deadlock, and two removals of one User's last ways to sign
		// in check them one after the other.
		held, err := users.LockIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		row, err := tx.Passkey.Query().Where(passkey.ID(id), passkey.UserID(userID)).Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return ErrPasskeyNotFound
			}
			return err
		}
		others, err := tx.Passkey.Query().Where(passkey.UserID(userID), passkey.IDNEQ(id)).Exist(ctx)
		if err != nil {
			return err
		}
		if held.PasswordHash == "" && !others {
			linked, err := tx.UserIdentity.Query().Where(useridentity.UserID(userID)).Exist(ctx)
			if err != nil {
				return err
			}
			if !linked {
				return ErrLastSignIn
			}
		}
		if err := tx.Passkey.DeleteOneID(id).Exec(ctx); err != nil {
			return err
		}
		saved, err := users.BumpSessionEpochIn(ctx, tx, userID)
		if err != nil {
			return err
		}
		removal = Removal{User: saved, Passkey: row, NoneLeft: !others}
		return pub.Publish(ctx, &events.PasskeyChange{Action: events.NameUserPasskeyRemoved, UserID: held.ID, Email: held.Email, Name: row.Name})
	})
	if err != nil {
		return Removal{}, err
	}
	return removal, nil
}

// Begun is a ceremony's begin step: the options for the browser (JSON) and the sealed ceremony
// cookie value its finish step presents.
type Begun struct {
	Options  []byte
	Ceremony string
}

// BeginRegistration starts adding a Passkey to u: the creation options for the browser and the
// sealed ceremony cookie value. It writes nothing. The options exclude the User's existing
// Passkeys, so one authenticator is not registered twice.
func (p *Module) BeginRegistration(ctx context.Context, u *ent.User) (Begun, error) {
	if !p.Available() {
		return Begun{}, ErrPasskeysUnavailable
	}
	rows, err := p.List(ctx, u.ID)
	if err != nil {
		return Begun{}, err
	}
	exclude := make([]protocol.CredentialDescriptor, len(rows))
	for i, r := range rows {
		exclude[i] = protocol.CredentialDescriptor{Type: protocol.PublicKeyCredentialType, CredentialID: r.CredentialID}
	}
	creation, session, err := p.party.wa.BeginRegistration(waUser{u}, webauthn.WithExclusions(exclude))
	if err != nil {
		return Begun{}, err
	}
	return p.begun(PurposeRegistration, u, creation.Response, session)
}

// begun is the Begun of a ceremony for purpose: its browser options and its session sealed for u
// (nil for a usernameless login).
func (p *Module) begun(purpose ceremonyPurpose, u *ent.User, options any, session *webauthn.SessionData) (Begun, error) {
	raw, err := json.Marshal(options)
	if err != nil {
		return Begun{}, err
	}
	ceremony, err := p.seal(purpose, u, session)
	if err != nil {
		return Begun{}, err
	}
	return Begun{Options: raw, Ceremony: ceremony}, nil
}

// Registration is the finish of a Passkey's registration: the proof of identity it takes, the
// ceremony cookie its begin step set, the authenticator's response (the browser's
// RegistrationResponseJSON) and the name the User gives the Passkey.
type Registration struct {
	Proof    users.IdentityProof
	Ceremony string
	Response []byte
	Name     string
}

// FinishRegistration completes the ceremony begun for the User u and stores the Passkey under its
// name (checked first), once the User proves their identity with in.Proof
// (users.Module.ProveIdentity). One transaction spends the
// challenge, stores the Passkey and publishes events.PasskeyChange (the owner is told, ADR 0105);
// it returns the stored Passkey. The session
// epoch stays: a new way to sign in takes nothing from the sessions already open (ADR 0052).
//
// A ceremony that cannot finish is ErrRegistrationFailed, wrapping its cause: ErrCeremony for an
// unusable cookie, ErrChallengeSpent when the ceremony already finished, ErrPasskeyRefused when
// the response is not acceptable (including an authenticator that did not verify the user).
func (p *Module) FinishRegistration(ctx context.Context, u *ent.User, in Registration) (*ent.Passkey, error) {
	name, err := validation.Name(in.Name)
	if err != nil {
		return nil, err
	}
	if err := p.users.ProveIdentity(ctx, u, in.Proof); err != nil {
		return nil, err
	}
	row, err := p.finishRegistration(ctx, u.ID, in.Ceremony, in.Response, name)
	if CeremonyFailed(err) {
		return nil, fmt.Errorf("%w: %w", ErrRegistrationFailed, err)
	}
	return row, err
}

func (p *Module) finishRegistration(ctx context.Context, userID int64, ceremony string, response []byte, name string) (*ent.Passkey, error) {
	if !p.Available() {
		return nil, ErrPasskeysUnavailable
	}
	claims, session, err := p.open(ceremony, PurposeRegistration)
	if err != nil || claims.UID != userID {
		return nil, ErrCeremony
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPasskeyRefused, err)
	}
	var row *ent.Passkey
	err = p.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if u.SessionEpoch != claims.Epoch {
			return ErrCeremony
		}
		cred, err := p.party.wa.CreateCredential(waUser{u}, *session, parsed)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrPasskeyRefused, err)
		}
		if err := p.spend(ctx, tx, session.Challenge); err != nil {
			return err
		}
		create := tx.Passkey.Create().
			SetUserID(userID).
			SetCredentialID(cred.ID).
			SetPublicKey(cred.PublicKey).
			SetSignCount(int64(cred.Authenticator.SignCount)).
			SetBackupEligible(cred.Flags.BackupEligible).
			SetBackupState(cred.Flags.BackupState).
			SetAaguid(cred.Authenticator.AAGUID).
			SetAttestationType(cred.AttestationType).
			SetName(name)
		if len(cred.Transport) > 0 {
			transports := make([]string, len(cred.Transport))
			for i, t := range cred.Transport {
				transports[i] = string(t)
			}
			create.SetTransports(transports)
		}
		if err := constraints.Write(ctx, tx, func() error {
			var cerr error
			row, cerr = create.Save(ctx)
			return cerr
		}); err != nil {
			return err
		}
		return pub.Publish(ctx, &events.PasskeyChange{Action: events.NameUserPasskeyAdded, UserID: u.ID, Email: u.Email, Name: row.Name})
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// spend records a challenge as used; ErrChallengeSpent when it already was.
func (p *Module) spend(ctx context.Context, tx *ent.Client, challenge string) error {
	return constraints.Write(ctx, tx, func() error {
		return tx.PasskeyChallenge.Create().SetChallenge(challenge).Exec(ctx)
	})
}

// HeldIn reports whether the User holds a Passkey, read on the transaction tx.
func (p *Module) HeldIn(ctx context.Context, tx *ent.Client, userID int64) (bool, error) {
	return tx.Passkey.Query().Where(passkey.UserID(userID)).Exist(ctx)
}

// RemoveAllIn removes every Passkey of the User on the transaction tx. It is part of the Second
// factor reset that internal/domain/identity/secondfactors runs after its check (who resets, and
// that there is something to clear), in the transaction that also clears the factor and ends the
// User's sessions.
func (p *Module) RemoveAllIn(ctx context.Context, tx *ent.Client, userID int64) error {
	_, err := tx.Passkey.Delete().Where(passkey.UserID(userID)).Exec(ctx)
	return err
}

// PruneSpentChallenges deletes the spent challenges recorded more than SpentChallengeRetention ago,
// longer than a ceremony can live, and returns how many went.
func (p *Module) PruneSpentChallenges(ctx context.Context) (int, error) {
	cutoff := p.now().Add(-SpentChallengeRetention)
	return p.ent.PasskeyChallenge.Delete().Where(passkeychallenge.CreatedAtLT(cutoff)).Exec(ctx)
}

// ceremonyClaims is the sealed ceremony cookie: the library's session data, the User it was
// begun for (0 for a usernameless login), and that User's session epoch at `begin`.
type ceremonyClaims struct {
	jwt.RegisteredClaims

	Purpose ceremonyPurpose `json:"p"`
	UID     int64           `json:"u,omitempty"`
	Epoch   int64           `json:"e,omitempty"`
	Session json.RawMessage `json:"s"`
}

// seal signs the ceremony for the cookie. u is nil for a usernameless login.
func (p *Module) seal(purpose ceremonyPurpose, u *ent.User, session *webauthn.SessionData) (string, error) {
	raw, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	c := ceremonyClaims{
		ExpiresAt: jwt.NewNumericDate(p.now().Add(CeremonyTTL)),
		Purpose:   purpose,
		Session:   raw,
	}
	if u != nil {
		c.UID, c.Epoch = u.ID, u.SessionEpoch
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, &c).SignedString(p.party.key)
}

// open verifies the cookie for purpose and returns its claims and session data.
func (p *Module) open(raw string, purpose ceremonyPurpose) (*ceremonyClaims, *webauthn.SessionData, error) {
	var c ceremonyClaims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return p.party.key, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(p.now))
	if err != nil || c.Purpose != purpose {
		return nil, nil, ErrCeremony
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(c.Session, &session); err != nil {
		return nil, nil, ErrCeremony
	}
	return &c, &session, nil
}

// waUser is a User as the WebAuthn library sees them.
type waUser struct{ u *ent.User }

func (w waUser) WebAuthnID() []byte                         { return UserHandle(w.u.ID) }
func (w waUser) WebAuthnName() string                       { return w.u.Email }
func (w waUser) WebAuthnDisplayName() string                { return w.u.Name }
func (w waUser) WebAuthnCredentials() []webauthn.Credential { return nil }
