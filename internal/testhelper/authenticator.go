package testhelper

import (
	"encoding/base64"
	"net/url"
	"testing"

	"github.com/descope/virtualwebauthn"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// Authenticator is a virtual WebAuthn authenticator and the browser in front of it (ADR 0051):
// it answers the options the server hands out the way a platform authenticator would, on
// descope/virtualwebauthn's attestation and assertion builders, so no CBOR or COSE is built by
// hand. It holds the credentials it created, each with the user handle the server gave it.
//
// A test drives a ceremony through the /site client: it posts `begin`, hands the options JSON
// from the response to Attest (registration) or Assert (login), and posts the returned JSON to
// `finish`. The same options and response bytes can be posted twice to test a replay.
type Authenticator struct {
	t    *testing.T
	rp   virtualwebauthn.RelyingParty
	auth virtualwebauthn.Authenticator
	held []heldCredential
}

type heldCredential struct {
	cred   virtualwebauthn.Credential
	handle []byte
}

// AuthenticatorOption tunes a virtual authenticator.
type AuthenticatorOption func(*virtualwebauthn.AuthenticatorOptions)

// WithoutUserVerification makes the authenticator skip the biometric or PIN check, so the
// responses it builds carry no user-verified flag (a security key without a PIN).
func WithoutUserVerification() AuthenticatorOption {
	return func(o *virtualwebauthn.AuthenticatorOptions) { o.UserNotVerified = true }
}

// NewAuthenticator is a virtual authenticator for the env's Instance: its relying party id and
// origin come from the env's SPHERICON_APP_URL, never a literal, so it fits every checkout.
func (env *TestEnv) NewAuthenticator(t *testing.T, opts ...AuthenticatorOption) *Authenticator {
	t.Helper()
	u, err := url.Parse(env.AppURL)
	require.NoError(t, err)
	options := virtualwebauthn.AuthenticatorOptions{}
	for _, o := range opts {
		o(&options)
	}
	return &Authenticator{
		t:    t,
		rp:   virtualwebauthn.RelyingParty{ID: u.Hostname(), Name: secondfactor.Issuer, Origin: u.Scheme + "://" + u.Host},
		auth: virtualwebauthn.NewAuthenticatorWithOptions(options),
	}
}

// FixturePasskey is a virtual authenticator that already holds the committed fixture Passkey
// (fixtures.PasskeyPatLaptop: Pat's laptop), signing with the private key the fixture states.
func (env *TestEnv) FixturePasskey(t *testing.T, opts ...AuthenticatorOption) *Authenticator {
	t.Helper()
	return env.fixtureAuthenticator(t, fixtures.PasskeyPatLaptopPrivateKey, fixtures.PasskeyPatLaptopCredentialID, fixtures.PasskeyPatID, opts)
}

// FixturePegPasskey is a virtual authenticator that already holds Peg's fixture Passkey
// (fixtures.PasskeyPegKey), the only way the password-less Peg signs in.
func (env *TestEnv) FixturePegPasskey(t *testing.T, opts ...AuthenticatorOption) *Authenticator {
	t.Helper()
	return env.fixtureAuthenticator(t, fixtures.PasskeyPegKeyPrivateKey, fixtures.PasskeyPegKeyCredentialID, fixtures.PasskeyOnlyPegID, opts)
}

// FixtureUnverifiedPasskey is a virtual authenticator that already holds Una's fixture Passkey
// (fixtures.PasskeyUnverifiedUnaKey): her email was never verified, so the gate stops her login.
func (env *TestEnv) FixtureUnverifiedPasskey(t *testing.T, opts ...AuthenticatorOption) *Authenticator {
	t.Helper()
	return env.fixtureAuthenticator(t, fixtures.PasskeyUnverifiedUnaKeyPrivateKey, fixtures.PasskeyUnverifiedUnaKeyCredentialID, fixtures.PasskeyUnverifiedUnaID, opts)
}

func (env *TestEnv) fixtureAuthenticator(t *testing.T, privateKey, credentialID string, userID int64, opts []AuthenticatorOption) *Authenticator {
	t.Helper()
	a := env.NewAuthenticator(t, opts...)
	key, err := base64.StdEncoding.DecodeString(privateKey)
	require.NoError(t, err)
	id, err := base64.RawURLEncoding.DecodeString(credentialID)
	require.NoError(t, err)
	cred := virtualwebauthn.NewCredentialWithImportedKey(virtualwebauthn.KeyTypeEC2, key)
	cred.ID = id
	a.held = append(a.held, heldCredential{cred: cred, handle: secondfactor.UserHandle(userID)})
	return a
}

// Attest answers a registration ceremony's creation options (the JSON `begin` returned): it
// creates a credential and returns the response a browser posts to `finish` (the W3C
// RegistrationResponseJSON). The authenticator keeps the credential from now on, as a real one
// does whether or not the server accepts it. It fails the test when the options exclude a
// credential this authenticator already holds (a browser refuses to register it twice).
func (a *Authenticator) Attest(optionsJSON []byte) []byte {
	a.t.Helper()
	options, err := virtualwebauthn.ParseAttestationOptions(string(optionsJSON))
	require.NoError(a.t, err)
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	for _, h := range a.held {
		require.False(a.t, h.cred.IsExcludedForAttestation(*options), "the options exclude a credential this authenticator holds")
	}
	response := virtualwebauthn.CreateAttestationResponse(a.rp, a.auth, cred, *options)
	a.held = append(a.held, heldCredential{cred: cred, handle: []byte(options.UserID)})
	return []byte(response)
}

// Assert answers a login ceremony's request options: it picks a held credential (one of the
// options' allowed credentials, or the first it holds for a usernameless login, where the list is
// empty), counts the use, and returns the response a browser posts to `finish` (the W3C
// AuthenticationResponseJSON), carrying the credential's user handle.
func (a *Authenticator) Assert(optionsJSON []byte) []byte {
	a.t.Helper()
	options, err := virtualwebauthn.ParseAssertionOptions(string(optionsJSON))
	require.NoError(a.t, err)
	i := a.pick(options)
	require.GreaterOrEqual(a.t, i, 0, "this authenticator holds no credential the options allow")
	h := &a.held[i]
	h.cred.Counter++
	auth := a.auth
	auth.Options.UserHandle = h.handle
	return []byte(virtualwebauthn.CreateAssertionResponse(a.rp, auth, h.cred, *options))
}

func (a *Authenticator) pick(options *virtualwebauthn.AssertionOptions) int {
	if len(options.AllowCredentials) == 0 {
		if len(a.held) == 0 {
			return -1
		}
		return 0
	}
	for i := range a.held {
		if a.held[i].cred.IsAllowedForAssertion(*options) {
			return i
		}
	}
	return -1
}

// CredentialID is the base64url id of the i-th credential this authenticator holds, the form
// the server's responses and the fixture catalog use.
func (a *Authenticator) CredentialID(i int) string {
	return base64.RawURLEncoding.EncodeToString(a.held[i].cred.ID)
}
