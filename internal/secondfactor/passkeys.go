package secondfactor

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/passkey"
	"github.com/getsphericon/sphericon/ent/passkeychallenge"
	"github.com/getsphericon/sphericon/ent/useridentity"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
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
	ErrCeremony = errors.New("secondfactor: passkey ceremony cookie unusable")
	// ErrChallengeSpent: the ceremony already finished once.
	ErrChallengeSpent = errors.New("secondfactor: passkey challenge already spent")
	// ErrPasskeysUnavailable: the Instance's public URL has no host name a Passkey can be bound
	// to (an IP address), so no Passkey can be registered or used on it.
	ErrPasskeysUnavailable = validation.NotFound("secondfactor: passkeys need a host name, not an IP address", i18n.ErrorsPasskeysUnavailable)
	// ErrPasskeyRefused: the authenticator's response is not acceptable: malformed, not
	// signed for this challenge or origin, made without user verification, or for a
	// credential that is already registered.
	ErrPasskeyRefused = errors.New("secondfactor: passkey response refused")
	// ErrPasskeyNotFound: no Passkey of that id belongs to the User (another User's counts as
	// none).
	ErrPasskeyNotFound = validation.NotFound("secondfactor: passkey not found", i18n.ErrorsPasskeyNotFound)
	// ErrPasskeyNameEmpty: the name given to a Passkey is empty once trimmed.
	ErrPasskeyNameEmpty = validation.RefuseField("name", "secondfactor: passkey name is empty", i18n.ErrorsPasskeyNameInvalid)
	// ErrRegistrationFailed: a registration ceremony that cannot finish (CeremonyFailed); the
	// User starts it again.
	ErrRegistrationFailed = validation.Refuse("secondfactor: passkey registration failed", i18n.ErrorsPasskeyRegistrationFailed)
	// ErrLastSignIn: the Passkey is the last way the User can sign in: no password, no Linked
	// identity and no other Passkey.
	ErrLastSignIn = validation.Conflict("secondfactor: the passkey is the last way to sign in", i18n.ErrorsPasskeyLastSignIn)
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

// Passkeys registers and checks a User's Passkeys (ADR 0052). The credentials are
// discoverable and user verification is required; attestation is not requested. The
// relying party id is the host of the Instance's public URL. A Passkey belongs to the
// User, so it works on the raw *ent.Client.
type Passkeys struct {
	ent *ent.Client
	bus *events.Bus
	wa  *webauthn.WebAuthn
	key []byte
	now func() time.Time
}

// NewPasskeys builds the module for the Instance at appURL (SPHERICON_APP_URL): its host
// is the relying party id and its origin the only one a ceremony may come from. An Instance
// served on an IP address gets a module that is not Available (a relying party id must be a
// domain), so booting never fails over it. secret is
// the Instance's JWT secret, from which the ceremony cookie's key is derived. now is the
// clock the cookie's expiry is checked against (nil means time.Now).
func NewPasskeys(client *ent.Client, bus *events.Bus, appURL, secret string, now func() time.Time) (*Passkeys, error) {
	u, err := url.Parse(appURL)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("passkeys: the app URL %q has no host", appURL)
	}
	if now == nil {
		now = time.Now
	}
	if net.ParseIP(u.Hostname()) != nil {
		return &Passkeys{ent: client, bus: bus, now: now}, nil
	}
	required := true
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                  u.Hostname(),
		RPDisplayName:         Issuer,
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
		return nil, fmt.Errorf("passkeys: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("passkey_ceremony"))
	return &Passkeys{ent: client, bus: bus, wa: wa, key: mac.Sum(nil), now: now}, nil
}

// Available reports whether Passkeys work on this Instance: its public URL has a host name.
func (p *Passkeys) Available() bool { return p.wa != nil }

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

// PasskeyName is the name a User gives a Passkey, trimmed; ErrPasskeyNameEmpty when nothing is
// left. The contract (PasskeyName) already bounds the length.
func PasskeyName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ErrPasskeyNameEmpty
	}
	return trimmed, nil
}

// List returns the User's Passkeys, oldest first.
func (p *Passkeys) List(ctx context.Context, userID int64) ([]*ent.Passkey, error) {
	return p.ent.Passkey.Query().Where(passkey.UserID(userID)).Order(passkey.ByID()).All(ctx)
}

// Rename gives the User's Passkey id a new name, trimmed (PasskeyName); ErrPasskeyNotFound when
// it is not theirs.
func (p *Passkeys) Rename(ctx context.Context, userID, id int64, name string) (*ent.Passkey, error) {
	name, err := PasskeyName(name)
	if err != nil {
		return nil, err
	}
	row, err := p.ent.Passkey.Query().Where(passkey.ID(id), passkey.UserID(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrPasskeyNotFound
	}
	if err != nil {
		return nil, err
	}
	return p.ent.Passkey.UpdateOneID(row.ID).SetName(name).Save(ctx)
}

// Remove deletes the User's Passkey id and bumps the session epoch, ending every session of the
// User, in one transaction. It returns the saved User, for reissuing the acting session, and the
// removed Passkey. The caller has proven the User's identity. ErrPasskeyNotFound when the
// Passkey is not theirs; ErrLastSignIn when it is the last way the User can sign in.
func (p *Passkeys) Remove(ctx context.Context, userID, id int64) (*ent.User, *ent.Passkey, error) {
	var (
		saved *ent.User
		row   *ent.Passkey
	)
	err := p.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if row, err = tx.Passkey.Query().Where(passkey.ID(id), passkey.UserID(userID)).Only(ctx); err != nil {
			if ent.IsNotFound(err) {
				return ErrPasskeyNotFound
			}
			return err
		}
		if u.PasswordHash == "" {
			others, err := tx.Passkey.Query().Where(passkey.UserID(userID), passkey.IDNEQ(id)).Exist(ctx)
			if err != nil {
				return err
			}
			linked, err := tx.UserIdentity.Query().Where(useridentity.UserID(userID)).Exist(ctx)
			if err != nil {
				return err
			}
			if !others && !linked {
				return ErrLastSignIn
			}
		}
		if err := tx.Passkey.DeleteOneID(id).Exec(ctx); err != nil {
			return err
		}
		saved, err = tx.User.UpdateOneID(userID).AddSessionEpoch(1).Save(ctx)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return saved, row, nil
}

// BeginRegistration starts adding a Passkey to u: the creation options for the browser (JSON)
// and the sealed ceremony cookie value. It writes nothing. The options exclude the User's
// existing Passkeys, so one authenticator is not registered twice.
func (p *Passkeys) BeginRegistration(ctx context.Context, u *ent.User) (options []byte, ceremony string, err error) {
	if !p.Available() {
		return nil, "", ErrPasskeysUnavailable
	}
	rows, err := p.List(ctx, u.ID)
	if err != nil {
		return nil, "", err
	}
	exclude := make([]protocol.CredentialDescriptor, len(rows))
	for i, r := range rows {
		exclude[i] = protocol.CredentialDescriptor{Type: protocol.PublicKeyCredentialType, CredentialID: r.CredentialID}
	}
	creation, session, err := p.wa.BeginRegistration(waUser{u}, webauthn.WithExclusions(exclude))
	if err != nil {
		return nil, "", err
	}
	if options, err = json.Marshal(creation.Response); err != nil {
		return nil, "", err
	}
	if ceremony, err = p.seal(PurposeRegistration, u, session); err != nil {
		return nil, "", err
	}
	return options, ceremony, nil
}

// FinishRegistration completes the ceremony begun under cookie for User userID, with the
// authenticator's response (the browser's RegistrationResponseJSON), and stores the Passkey
// under name. The caller has proven the User's identity. One transaction spends the
// challenge and stores the Passkey; it returns the User and the stored Passkey. The session
// epoch stays: a new way to sign in takes nothing from the sessions already open (ADR 0052).
//
// A ceremony that cannot finish is ErrRegistrationFailed, wrapping its cause: ErrCeremony for an
// unusable cookie, ErrChallengeSpent when the ceremony already finished, ErrPasskeyRefused when
// the response is not acceptable (including an authenticator that did not verify the user).
func (p *Passkeys) FinishRegistration(ctx context.Context, userID int64, ceremony string, response []byte, name string) (*ent.User, *ent.Passkey, error) {
	saved, row, err := p.finishRegistration(ctx, userID, ceremony, response, name)
	if CeremonyFailed(err) {
		return nil, nil, fmt.Errorf("%w: %w", ErrRegistrationFailed, err)
	}
	return saved, row, err
}

func (p *Passkeys) finishRegistration(ctx context.Context, userID int64, ceremony string, response []byte, name string) (*ent.User, *ent.Passkey, error) {
	if !p.Available() {
		return nil, nil, ErrPasskeysUnavailable
	}
	claims, session, err := p.open(ceremony, PurposeRegistration)
	if err != nil || claims.UID != userID {
		return nil, nil, ErrCeremony
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrPasskeyRefused, err)
	}
	var (
		saved *ent.User
		row   *ent.Passkey
	)
	err = p.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		u, err := tx.User.Get(ctx, userID)
		if err != nil {
			return err
		}
		if u.SessionEpoch != claims.Epoch {
			return ErrCeremony
		}
		cred, err := p.wa.CreateCredential(waUser{u}, *session, parsed)
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
		if row, err = create.Save(ctx); err != nil {
			if ent.IsConstraintError(err) {
				return fmt.Errorf("%w: credential already registered", ErrPasskeyRefused)
			}
			return err
		}
		saved = u
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return saved, row, nil
}

// spend records a challenge as used; ErrChallengeSpent when it already was.
func (p *Passkeys) spend(ctx context.Context, tx *ent.Client, challenge string) error {
	err := tx.PasskeyChallenge.Create().SetChallenge(challenge).Exec(ctx)
	if ent.IsConstraintError(err) {
		return ErrChallengeSpent
	}
	return err
}

// PruneSpentChallenges deletes the spent challenges recorded before cutoff and returns how many
// went. The cutoff must be older than a ceremony can live (SpentChallengeRetention is).
func PruneSpentChallenges(ctx context.Context, client *ent.Client, cutoff time.Time) (int, error) {
	return client.PasskeyChallenge.Delete().Where(passkeychallenge.CreatedAtLT(cutoff)).Exec(ctx)
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
func (p *Passkeys) seal(purpose ceremonyPurpose, u *ent.User, session *webauthn.SessionData) (string, error) {
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
	return jwt.NewWithClaims(jwt.SigningMethodHS256, &c).SignedString(p.key)
}

// open verifies the cookie for purpose and returns its claims and session data.
func (p *Passkeys) open(raw string, purpose ceremonyPurpose) (*ceremonyClaims, *webauthn.SessionData, error) {
	var c ceremonyClaims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return p.key, nil },
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
