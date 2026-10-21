package testhelper_test

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/passkey"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// fixtureUser is the fixture User as the WebAuthn library sees them, holding the stored Passkey.
type fixtureUser struct{ credentials []webauthn.Credential }

func (fixtureUser) WebAuthnID() []byte                           { return secondfactor.UserHandle(fixtures.PasskeyPatID) }
func (fixtureUser) WebAuthnName() string                         { return fixtures.PasskeyPatEmail }
func (fixtureUser) WebAuthnDisplayName() string                  { return fixtures.PasskeyPatName }
func (u fixtureUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// The fixture Passkey's stored public key is the one its private key signs for: the test
// authenticator logs in with it against a stock WebAuthn relying party.
func TestTheFixturePasskeyAssertsAgainstItsStoredPublicKey(t *testing.T) {
	env := testhelper.Setup(t)
	appURL, err := url.Parse(env.AppURL)
	require.NoError(t, err)
	rp, err := webauthn.New(&webauthn.Config{
		RPID: appURL.Hostname(), RPDisplayName: "sphericon", RPOrigins: []string{appURL.Scheme + "://" + appURL.Host},
	})
	require.NoError(t, err)
	row, err := env.DB.Passkey.Query().Where(passkey.UserID(fixtures.PasskeyPatID)).Only(t.Context())
	require.NoError(t, err)
	user := fixtureUser{credentials: []webauthn.Credential{{
		ID: row.CredentialID, PublicKey: row.PublicKey,
		Flags: webauthn.CredentialFlags{BackupEligible: row.BackupEligible, BackupState: row.BackupState},
	}}}
	assertion, session, err := rp.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	require.NoError(t, err)
	options, err := json.Marshal(assertion.Response)
	require.NoError(t, err)
	response := env.FixturePasskey(t).Assert(options)
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	require.NoError(t, err)

	found, credential, err := rp.ValidatePasskeyLogin(func(_, userHandle []byte) (webauthn.User, error) {
		assert.Equal(t, secondfactor.UserHandle(fixtures.PasskeyPatID), userHandle)
		return user, nil
	}, *session, parsed)

	require.NoError(t, err)
	assert.Equal(t, fixtures.PasskeyPatName, found.WebAuthnDisplayName())
	assert.Equal(t, row.CredentialID, credential.ID)
}
