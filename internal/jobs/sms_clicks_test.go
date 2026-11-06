package jobs_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/trackedlink"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outbound"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Clicks in SMS (ADR 0083): every link of a sent text is a short link on the Tracking domain or the
// Instance's host, unless the message turns link tracking off, and following one records
// sms.clicked before it redirects. A Test send shows the same form.

const (
	smsOffer = "https://shop.test/offer?a=1&b=2"
	smsCart  = "https://shop.test/cart"
	// whatsAppUA is WhatsApp fetching a link for its preview; iMessageUA is iMessage's preview.
	whatsAppUA = "WhatsApp/2.23.20.0 A"
	iMessageUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_1) AppleWebKit/601.2.4 (KHTML, like Gecko) Version/9.0.1 Safari/601.2.4 facebookexternalhit/1.1 Facebot Twitterbot/1.0"
)

// smsShortLink is a short tracked link inside a text: its `/c/<token>` path is the submatch.
var smsShortLink = regexp.MustCompile(`https?://[^/\s]+(/c/[A-Za-z0-9_-]{11})\b`)

// smsShortPaths are the `/c/<token>` paths of the links of the only text Twilio got.
func smsShortPaths(t *testing.T, env *testhelper.TestEnv) []string {
	t.Helper()
	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	links := smsShortLink.FindAllStringSubmatch(sent[0].Body, -1)
	paths := make([]string, 0, len(links))
	for _, m := range links {
		paths = append(paths, m[1])
	}
	return paths
}

// Lacuna's draft "Sale: <offer> or your cart <cart>. Stop: <unsubscribe link>" goes to Textham, whose
// one number that can be texted is Bea's.
func TestAnSMSBroadcastsLinksGoOutAsShortTrackedLinksAndAPersonsClickIsRecorded(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTwilio(t)
	id := int64(fixtures.BroadcastLacunaSmsLinksID)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	sent := env.Twilio.Messages()
	body := sent[0].Body
	assert.NotContains(t, body, "shop.test", "no link goes out as written")
	assert.Contains(t, body, "/e/u/", "the opt-out link goes straight to the unsubscribe page")
	paths := smsShortPaths(t, env)
	require.Len(t, paths, 2)
	assert.NotEqual(t, paths[0], paths[1])
	linkLength, err := env.OutboundModule().ShortLinkLength(ctx, env.DB.Scoped(fixtures.LacunaID))
	require.NoError(t, err)
	links := smsShortLink.FindAllString(body, -1)
	lengths := lo.Map(links, func(link string, _ int) int { return len(link) })
	assert.Equal(t, []int{linkLength, linkLength}, lengths, "the counter's length is the length that goes out")
	assert.Regexp(t, `^Sale: https?://\S+/c/\S{11} or your cart https?://\S+/c/\S{11}\. Stop: `, body,
		"the punctuation after a link stays text")
	clicked := func() bool {
		return env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id)).OnlyX(ctx).ClickedAt != nil
	}

	for _, preview := range []string{whatsAppUA, iMessageUA} {
		resp := follow(t, env, paths[0], preview)
		assert.Equal(t, http.StatusFound, resp.Code, "a preview fetch is redirected like anyone")
		assert.Equal(t, smsOffer, resp.Header().Get("Location"))
	}
	clickedAfterPreviews := clicked()
	assert.False(t, clickedAfterPreviews, "a preview fetch sets no rollup")

	resp := follow(t, env, paths[1], browserUA)

	assert.Equal(t, http.StatusFound, resp.Code)
	assert.Equal(t, smsCart, resp.Header().Get("Location"))
	clickedAfterClick := clicked()
	assert.True(t, clickedAfterClick, "a person's click sets the recipient's rollup")
	report, err := broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule()).Report(ctx, env.DB.Scoped(fixtures.LacunaID), id)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Clicked, "the SMS report shows clicked")
	clicks := env.QueuedEvents(t, events.NameSmsClicked)
	require.Len(t, clicks, 3)
	assert.True(t, clicks[0].(*events.SmsClicked).Machine)
	assert.True(t, clicks[1].(*events.SmsClicked).Machine)
	human := clicks[2].(*events.SmsClicked)
	assert.False(t, human.Machine)
	assert.Equal(t, id, human.BroadcastID)
	assert.EqualValues(t, fixtures.ContactLacunaBeaID, human.ContactID)
	assert.Equal(t, smsCart, human.URL)
}

// Lacuna's untracked draft "Sale: <offer> or <cart>" goes to Textham, whose one number that can be
// texted is Bea's.
func TestAnSMSBroadcastWithLinkTrackingOffGoesOutAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTwilio(t)
	id := int64(fixtures.BroadcastLacunaSmsUntrackedID)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, "Sale: "+smsOffer+" or "+smsCart, sent[0].Body)
	tracked := env.DB.TrackedLink.Query().Where(trackedlink.HasOutboundMessageWith(outboundmessage.BroadcastID(id))).CountX(ctx)
	assert.Zero(t, tracked, "no short link was made")
}

// Lacuna's draft "Hi {{ first_name }}, see <offer>".
func TestATestSendOfAnSMSBroadcastShowsTheShortLinksAndTheirClicksRecordNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTwilio(t)
	s := env.DB.Scoped(fixtures.LacunaID)
	id := int64(fixtures.BroadcastLacunaSmsOfferID)
	b := env.DB.Broadcast.GetX(ctx, id)
	mod := env.OutboundModule()

	_, err := mod.SendBroadcastTest(ctx, s, b, outbound.TestAddress{Phone: "+14155550199"}, nil)
	require.NoError(t, err)
	_, err = mod.SendBroadcastTest(ctx, s, b, outbound.TestAddress{Phone: "+14155550199"}, nil)
	require.NoError(t, err)

	sent := env.Twilio.Messages()
	require.Len(t, sent, 2)
	assert.Regexp(t, `^Hi Alex, see https?://\S+/c/\S{11}$`, sent[0].Body, "the same form a real send has")
	assert.Equal(t, sent[0].Body, sent[1].Body, "a later Test send reuses the link")
	path := smsShortLink.FindStringSubmatch(sent[0].Body)[1]
	resp := follow(t, env, path, browserUA)
	assert.Equal(t, http.StatusFound, resp.Code)
	assert.Equal(t, smsOffer, resp.Header().Get("Location"))
	smsClicked := env.QueuedCount(t, events.NameSmsClicked, nil)
	assert.Zero(t, smsClicked, "a Test send's click records nothing")
	messages := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id)).CountX(ctx)
	assert.Zero(t, messages, "a Test send records no message")
	unattached := env.DB.TrackedLink.Query().Where(trackedlink.OutboundMessageIDIsNil()).CountX(ctx)
	assert.Equal(t, 1, unattached)
}

// sms.clicked is a Trigger like email.clicked: a person's click on an SMS step's link enrolls the
// Contact with the click's URL, and a preview fetch starts nothing (ADR 0053, 0083). Fixture: the
// Automation the fixtures trigger on sms.clicked.
func TestAnSMSStepsLinkClickRecordsTheAutomationAndTriggersAnother(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.AcmeTwilio(t)
	sender := int64(fixtures.AutomationJobsSMSCartID)
	queueStep(t, env, fixtures.AutomationRunJobsSMSCartConfirmedID)
	runSteps(t, env)
	paths := smsShortPaths(t, env)
	require.Len(t, paths, 1)
	consume := func() []jobs.EvaluateTriggerArgs {
		envelopes := env.QueuedEnvelopes(t, events.NameSmsClicked)
		env.WorkAutomationJobOf(t, envelopes[len(envelopes)-1])
		return queuedArgs[jobs.EvaluateTriggerArgs](t, env, jobs.EvaluateTriggerArgs{}.Kind())
	}

	follow(t, env, paths[0], whatsAppUA)
	previewTriggers := consume()
	assert.Empty(t, previewTriggers, "the preview fetch starts nothing")

	follow(t, env, paths[0], browserUA)
	triggers := consume()
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, env.Bus).Work(ctx, job(triggers[0])))
	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationOnSmsClickedID), automationrun.ContactID(fixtures.ContactTexterConfirmedID)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.NameSmsClicked, *run.EventAction)
	assert.Equal(t, smsCart, run.EventProperties["url"])
	clicks := env.QueuedEvents(t, events.NameSmsClicked)
	last := clicks[len(clicks)-1].(*events.SmsClicked)
	assert.Equal(t, sender, last.AutomationID)
}

func TestAnSMSStepWithLinkTrackingOffSendsItsLinksAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	env.AcmeTwilio(t)
	queueStep(t, env, fixtures.AutomationRunJobsSMSCartUntrackedConfirmedID)

	runSteps(t, env)

	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, "Your cart: "+smsCart, sent[0].Body)
}
