package collect_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/starttoken"
	"github.com/getsphericon/sphericon/ent/visitor"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// identitySecret generates Acme's Identity secret as its owner does, in the settings.
func identitySecret(t *testing.T, env *testhelper.TestEnv) string {
	t.Helper()
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIdentitySecretRegenerate(t.Context(),
		siteapi.SiteIdentitySecretRegenerateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	secret, ok := res.Secret.Get()
	require.True(t, ok)
	return secret
}

// signed is an Identity token as the customer's server signs it.
func signed(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)
	return token
}

func identifyProven(t *testing.T, env *testhelper.TestEnv, in collectapi.CollectIdentifyInput) error {
	t.Helper()
	_, err := env.CollectAcme(t).CollectIdentifyCreate(t.Context(), &in)
	return err
}

// A proven Identify (ADR 0171) is what Identify was for the customer's own code after a sign-in: it
// fills the Contact's keys, writes its attributes, moves the Visitor from another Contact and hands
// the Contact the chats the Visitor started, recording telegram.bound for each.
func TestAProvenIdentifyFillsKeysMovesTheVisitorAndHandsOverItsChats(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)
	secret := identitySecret(t, env)
	identify(t, env, "dev-proven", fixtures.ContactAliceEmail)
	env.StartTelegram(t, bot, 1, 6101, startLink(t, env, "dev-proven").URL)

	err := identifyProven(t, env, collectapi.CollectIdentifyInput{
		VisitorId:     "dev-proven",
		Email:         collectapi.NewOptNilEmailAddress(fixtures.ContactBobEmail),
		IdentityToken: collectapi.NewOptNilString(signed(t, secret, jwt.MapClaims{"sub": "bob-1", "email": fixtures.ContactBobEmail, "exp": time.Now().Add(time.Hour).Unix()})),
		Traits:        collectapi.NewOptNilJsonObject(collectapi.JsonObject{"firstName": jx.Raw(`"Robert"`)}),
	})
	require.NoError(t, err)

	bob := env.DB.Contact.GetX(t.Context(), fixtures.ContactBobID)
	assert.Equal(t, "bob-1", lo.FromPtr(bob.SubjectID), "a proven key fills the Contact")
	assert.Equal(t, "Robert", lo.FromPtr(bob.FirstName))
	vis := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-proven")).OnlyX(t.Context())
	assert.EqualValues(t, fixtures.ContactBobID, lo.FromPtr(vis.ContactID), "the Visitor moves from Alice to Bob")
	chat := env.DB.Binding.Query().Where(binding.Destination("6101")).OnlyX(t.Context())
	assert.EqualValues(t, fixtures.ContactBobID, lo.FromPtr(chat.ContactID), "the Visitor's chat is Bob's")
	assert.Nil(t, chat.VisitorID)
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramBound, map[string]any{"contactId": fixtures.ContactBobID}))

	require.NoError(t, identifyProven(t, env, collectapi.CollectIdentifyInput{
		VisitorId:     "dev-proven",
		IdentityToken: collectapi.NewOptNilString(signed(t, secret, jwt.MapClaims{"email": fixtures.ContactBobEmail, "exp": time.Now().Add(time.Hour).Unix()})),
	}))
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramBound, map[string]any{"contactId": fixtures.ContactBobID}), "handed over once")
}

// A token that does not verify refuses the Identify; it is never read as a claim.
func TestAnIdentityTokenThatDoesNotVerifyRefusesTheIdentify(t *testing.T) {
	env := testhelper.Setup(t)
	hour := time.Now().Add(time.Hour).Unix()
	bob := jwt.MapClaims{"email": fixtures.ContactBobEmail, "exp": hour}
	env.DB.IdentitySecret.DeleteOneID(fixtures.IdentitySecretAcmeID).ExecX(t.Context())

	err := identifyProven(t, env, collectapi.CollectIdentifyInput{
		VisitorId: "dev-x", IdentityToken: collectapi.NewOptNilString(signed(t, "idsec_never-generated", bob)),
	})
	testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity, "a Workspace with no Identity secret")

	secret := identitySecret(t, env)
	for name, token := range map[string]string{
		"another secret": signed(t, "idsec_another", bob),
		"no expiry":      signed(t, secret, jwt.MapClaims{"email": fixtures.ContactBobEmail}),
		"expired":        signed(t, secret, jwt.MapClaims{"email": fixtures.ContactBobEmail, "exp": time.Now().Add(-time.Hour).Unix()}),
		"over a day":     signed(t, secret, jwt.MapClaims{"email": fixtures.ContactBobEmail, "exp": time.Now().Add(48 * time.Hour).Unix()}),
		"no key":         signed(t, secret, jwt.MapClaims{"exp": hour}),
		"not a token":    "garbage",
	} {
		err := identifyProven(t, env, collectapi.CollectIdentifyInput{VisitorId: "dev-x", IdentityToken: collectapi.NewOptNilString(token)})
		testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity, name)
	}

	err = identifyProven(t, env, collectapi.CollectIdentifyInput{
		VisitorId:     "dev-x",
		Email:         collectapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail),
		IdentityToken: collectapi.NewOptNilString(signed(t, secret, bob)),
	})
	testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity, "a key the token does not prove")
	assert.False(t, env.DB.Visitor.Query().Where(visitor.VisitorID("dev-x")).ExistX(t.Context()), "a refused Identify sees no Visitor")
}

// The tracker's start link with a token that proves a Contact is that Contact's: the chat that starts
// with it is the Contact's at once.
func TestAStartLinkWithAnIdentityTokenIsTheProvenContacts(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)
	secret := identitySecret(t, env)
	before := env.DB.StartToken.Query().IDsX(t.Context())

	_, err := env.CollectAcme(t).CollectTelegramStartLinksCreate(t.Context(), &collectapi.CollectTelegramStartLinkInput{
		VisitorId:     "dev-link",
		IdentityToken: collectapi.NewOptNilString(signed(t, secret, jwt.MapClaims{"email": fixtures.ContactBobEmail, "exp": time.Now().Add(time.Hour).Unix()})),
	})
	require.NoError(t, err)

	tok := env.DB.StartToken.Query().Where(starttoken.IDNotIn(before...)).OnlyX(t.Context())
	assert.EqualValues(t, fixtures.ContactBobID, lo.FromPtr(tok.ContactID))
	assert.Nil(t, tok.VisitorID)
}

// The Identity secret is read and regenerated by owners and admins only; a Workspace has none until
// one is generated, and regenerating replaces it.
func TestTheIdentitySecretIsAnOwnersAndAdminsAndRegeneratingReplacesIt(t *testing.T) {
	env := testhelper.Setup(t)
	params := siteapi.SiteIdentitySecretGetParams{WorkspaceId: fixtures.AcmePublicID}
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	env.DB.IdentitySecret.DeleteOneID(fixtures.IdentitySecretAcmeID).ExecX(t.Context())

	none, err := owner.SiteIdentitySecretGet(t.Context(), params)
	require.NoError(t, err)
	assert.False(t, none.Secret.IsSet())

	first := identitySecret(t, env)
	read, err := env.SiteActor(t, fixtures.AdminAliceEmail).SiteIdentitySecretGet(t.Context(), params)
	require.NoError(t, err)
	assert.Equal(t, first, read.Secret.Or(""))
	assert.NotEqual(t, first, identitySecret(t, env))

	_, err = env.SiteActor(t, fixtures.MemberMaryEmail).SiteIdentitySecretGet(t.Context(), params)
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
}
