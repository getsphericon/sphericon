package collect_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/visitor"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Link identity token (ADR 0160) binds the Visitor the tracker presents it with to the recipient's
// Contact as a claim binds (ADR 0171): only an anonymous Visitor or one already that Contact's, its
// anonymous Events stitched, none of its Bindings handed over. The collect call answers 204
// whatever it binds.

// linkToken is the Link identity token a tracked link's click gives contactID of Acme, for a message
// sent now.
func linkToken(t *testing.T, env *testhelper.TestEnv, contactID int64) string {
	t.Helper()
	token, err := env.Tracker.LinkIdentityToken(messagetoken.LinkIdentity{WorkspaceID: fixtures.AcmeID, ContactID: contactID}, time.Now())
	require.NoError(t, err)
	return token
}

func presentToken(t *testing.T, c *collectapi.Client, visitorID, token string) {
	t.Helper()
	require.NoError(t, c.CollectLinkIdentitiesCreate(t.Context(), &collectapi.CollectLinkIdentityInput{VisitorId: visitorID, Token: token}))
}

// visitorContact is the Contact Acme's Visitor is bound to, 0 while it is anonymous or not seen.
func visitorContact(t *testing.T, env *testhelper.TestEnv, visitorID string) int64 {
	t.Helper()
	vis, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID(visitorID)).Only(t.Context())
	if ent.IsNotFound(err) {
		return 0
	}
	require.NoError(t, err)
	return lo.FromPtr(vis.ContactID)
}

// An anonymous Visitor arriving with a valid token is bound to the Contact, and its earlier
// anonymous page views become the Contact's.
func TestALinkIdentityTokenBindsAnAnonymousVisitorAndStitchesItsPageViews(t *testing.T) {
	env := testhelper.Setup(t)
	const device = fixtures.EventAnonymousFirstVisitorID

	presentToken(t, env.CollectAcme(t), device, linkToken(t, env, fixtures.ContactBobID))

	bound := visitorContact(t, env, device)
	stitched := env.DB.Event.Query().Where(event.VisitorID(device)).OnlyX(t.Context())
	other := env.DB.Event.Query().Where(event.VisitorID(fixtures.EventAnonymousSecondVisitorID)).OnlyX(t.Context())
	assert.EqualValues(t, fixtures.ContactBobID, bound)
	assert.EqualValues(t, fixtures.ContactBobID, lo.FromPtr(stitched.ContactID))
	assert.Nil(t, other.ContactID, "another device stays anonymous")
}

// A Visitor of another Contact stays where it is.
func TestALinkIdentityTokenLeavesAVisitorOfAnotherContact(t *testing.T) {
	env := testhelper.Setup(t)
	env.IdentifyVisitor(t, "dev-alice", fixtures.ContactAliceEmail)

	presentToken(t, env.CollectAcme(t), "dev-alice", linkToken(t, env, fixtures.ContactBobID))

	bound := visitorContact(t, env, "dev-alice")
	assert.EqualValues(t, fixtures.ContactAliceID, bound)
}

// A Telegram chat started from the Visitor before the token binds it stays out of the Contact's
// Bindings: no telegram.bound is recorded and the Contact's card lists no chat.
func TestALinkIdentityTokenHandsTheContactNoneOfTheVisitorsChats(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)
	env.StartTelegram(t, bot, 1, 6201, startLink(t, env, "dev-chat").URL)

	presentToken(t, env.CollectAcme(t), "dev-chat", linkToken(t, env, fixtures.ContactBobID))

	bound := visitorContact(t, env, "dev-chat")
	chat := env.DB.Binding.Query().Where(binding.Destination("6201")).OnlyX(t.Context())
	bobsChats := contactChats(t, env, fixtures.ContactBobEmail)
	boundEvents := env.QueuedCount(t, systemevent.TelegramBound, nil)
	assert.EqualValues(t, fixtures.ContactBobID, bound)
	assert.Nil(t, chat.ContactID, "the chat stays the Visitor's")
	assert.Empty(t, bobsChats)
	assert.Zero(t, boundEvents)
}

// A wrong binding (a forwarded message) is corrected only by a proven Identify: a claim for another
// Contact leaves the Visitor where the token bound it, a proven one moves it.
func TestOnlyAProvenIdentifyMovesAVisitorATokenBound(t *testing.T) {
	env := testhelper.Setup(t)
	secret := identitySecret(t, env)
	presentToken(t, env.CollectAcme(t), "dev-forwarded", linkToken(t, env, fixtures.ContactBobID))

	env.IdentifyVisitor(t, "dev-forwarded", fixtures.ContactAliceEmail)
	claimed := visitorContact(t, env, "dev-forwarded")
	require.NoError(t, identifyProven(t, env, collectapi.CollectIdentifyInput{
		VisitorId:     "dev-forwarded",
		IdentityToken: collectapi.NewOptNilString(signed(t, secret, jwt.MapClaims{"email": fixtures.ContactAliceEmail, "exp": time.Now().Add(time.Hour).Unix()})),
	}))
	proven := visitorContact(t, env, "dev-forwarded")

	assert.EqualValues(t, fixtures.ContactBobID, claimed, "a claim leaves it")
	assert.EqualValues(t, fixtures.ContactAliceID, proven, "a proven Identify moves it")
}

// A token that does not stand binds nothing, and the call answers as for one that does.
func TestATokenThatDoesNotStandBindsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	bob := messagetoken.LinkIdentity{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactBobID}
	expired, err := env.Tracker.LinkIdentityToken(bob, time.Now().Add(-8*24*time.Hour))
	require.NoError(t, err)
	globex, err := env.Tracker.LinkIdentityToken(messagetoken.LinkIdentity{WorkspaceID: fixtures.GlobexID, ContactID: fixtures.ContactGlobexReaderID}, time.Now())
	require.NoError(t, err)
	forged, err := messagetoken.New("another-secret", "https://app.test", time.Now).LinkIdentityToken(bob, time.Now())
	require.NoError(t, err)
	unsubscribe, err := env.Tracker.UnsubscribeURL(linktarget.Unsubscribe{
		Channel: channels.Email, Source: "broadcasts", Destination: fixtures.ContactBobEmail, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactBobID,
	})
	require.NoError(t, err)
	confirm, err := env.Tracker.ConfirmURL(linktarget.Confirmation{Destination: fixtures.ContactBobEmail, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactBobID})
	require.NoError(t, err)
	origin := len(env.Tracker.Origin())

	for name, token := range map[string]string{
		"expired":          expired,
		"another's":        globex,
		"a bad signature":  forged,
		"an unsubscribe":   unsubscribe[origin+len("/e/u/"):],
		"a confirmation":   confirm[origin+len("/e/confirm/"):],
		"a gone Contact's": linkToken(t, env, 999999),
		"not a token":      "garbage",
	} {
		t.Run(name, func(t *testing.T) {
			presentToken(t, env.CollectAcme(t), fixtures.EventAnonymousFirstVisitorID, token)

			bound := visitorContact(t, env, fixtures.EventAnonymousFirstVisitorID)
			assert.Zero(t, bound)
		})
	}
}

// A request whose User-Agent names a known crawler binds nothing.
func TestALinkIdentityTokenFromACrawlerBindsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	crawler := env.CollectVia(t, testhelper.WithUserAgent("Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"))

	presentToken(t, crawler, fixtures.EventAnonymousFirstVisitorID, linkToken(t, env, fixtures.ContactBobID))

	bound := visitorContact(t, env, fixtures.EventAnonymousFirstVisitorID)
	assert.Zero(t, bound)
}
