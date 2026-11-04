package site_test

import (
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An SMS Broadcast on the site API (ADR 0080): its content is a plain text, it goes to the
// confirmed numbers of its Segment through the fake Twilio, its figures leave opened, delivered,
// bounced and complained null, and its Test send goes to a number the author types.

// createSMSBroadcast creates an SMS Broadcast through the account with text, to the Segment when
// one is given, and returns it.
func createSMSBroadcast(t *testing.T, c *siteapi.Client, account testhelper.TwilioAccount, text string, segment *siteapi.EntityId) *siteapi.SiteBroadcastResource {
	t.Helper()
	in := &siteapi.CreateBroadcastInput{
		Name: "Flash sale", Content: testhelper.SiteSMS(text),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(int64(account.Integration.ID))),
	}
	if segment != nil {
		in.SegmentId = siteapi.NewOptNilEntityId(*segment)
	}
	res, err := c.SiteBroadcastsCreate(t.Context(), in, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	b, ok := res.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", res)
	return b
}

// confirmNumber records the SMS Confirmation an API client asserts for the number.
func confirmNumber(t *testing.T, env *testhelper.TestEnv, number string) {
	t.Helper()
	_, err := env.ExternalScoped(t, "contacts:write").ConfirmationsCreate(t.Context(), &externalapi.CreateConfirmationInput{
		Channel: externalapi.ConsentChannelSMS, Destination: number,
		Wording: "Yes, text me offers from Acme. Reply STOP to opt out.", Source: "checkout form", Brand: "Acme",
	})
	require.NoError(t, err)
}

func TestSiteSMSBroadcastToASegmentTextsTheConfirmedNumbersAndReportsItsFigures(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	confirmNumber(t, env, fixtures.ContactTexterPhone)
	seg, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{
		Name:      "Bay Area numbers",
		Condition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"phone","operator":"beginsWith","value":"+1415555"}]}`),
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	segment, ok := seg.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", seg)
	made := createSMSBroadcast(t, c, account, "Hi {{ first_name }}, 20% off today", &segment.ID)

	sent, err := c.SiteBroadcastsSend(ctx, siteapi.SiteBroadcastsSendParams{WorkspaceId: fixtures.AcmePublicID, ID: made.ID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteBroadcastResource{}, sent)
	env.DeliverQueuedBroadcasts(t)
	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: made.ID})
	require.NoError(t, err)
	read, ok := got.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", got)

	assert.Equal(t, siteapi.BroadcastChannelSMS, made.Channel)
	content := testhelper.SiteSMSOf(made)
	assert.Equal(t, "Hi {{ first_name }}, 20% off today", content.Text)
	assert.Empty(t, made.Warnings, "email's lint does not apply to a text")
	texts := env.Twilio.Messages()
	require.Len(t, texts, 1, "the Segment's other phone has no Confirmation")
	assert.Equal(t, fixtures.ContactTexterPhone, texts[0].To)
	assert.Equal(t, "Hi Tess, 20% off today", texts[0].Body)
	assert.Equal(t, siteapi.SiteBroadcastStatusSent, read.Status)
	st := read.Stats
	assert.Equal(t, int32(1), st.RecipientsTotal)
	assert.Equal(t, int32(1), st.SentCount)
	assert.Zero(t, st.SkippedCount)
	assert.Zero(t, st.FailedCount)
	assert.Zero(t, st.PendingCount)
	assert.True(t, st.OpenedCount.Null)
	assert.Equal(t, siteapi.NewNilInt32(0), st.DeliveredCount, "SMS reports deliveries, none yet")
	assert.Equal(t, siteapi.NewNilInt32(0), st.UndeliveredCount)
	assert.True(t, st.BouncedCount.Null)
	assert.True(t, st.ComplainedCount.Null)
	assert.True(t, st.CapturedCount.Null)
	assert.True(t, st.DeliveryRate.Null)
	assert.True(t, st.OpenRate.Null)
	assert.True(t, st.ClickToOpenRate.Null)
}

// A Workspace with no SMS Integration has no SMS Broadcast to start: Globex has none.
func TestSiteSMSBroadcastIsRefusedWithoutAnSMSIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	res, err := c.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{Name: "Flash sale", Content: testhelper.SiteSMS("Hi")},
		siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.GlobexPublicID})

	require.NoError(t, err)
	refused, ok := res.(*siteapi.SiteBroadcastsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Value, "no integration of this channel")
}

func TestSiteSMSBroadcastTestSendGoesToTheTypedNumberAndLeavesNoMessage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	b := createSMSBroadcast(t, c, account, "Hi {{ first_name }}, stop: {{ unsubscribe_url }}", nil)
	testSend := func(in siteapi.TestSendBroadcastInput) siteapi.SiteBroadcastsTestSendRes {
		res, err := c.SiteBroadcastsTestSend(ctx, &in, siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
		require.NoError(t, err)
		return res
	}
	messagesBefore := env.DB.OutboundMessage.Query().CountX(ctx)

	sent := testSend(siteapi.TestSendBroadcastInput{Phone: siteapi.NewOptString("+1 (415) 555-0199")})
	invalid := testSend(siteapi.TestSendBroadcastInput{Phone: siteapi.NewOptString("415 555 0199")})
	emailOnly := testSend(siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress("qa@acme.test")})

	result, ok := sent.(*siteapi.SiteTestSendBroadcastResult)
	require.Truef(t, ok, "got %T", sent)
	assert.False(t, result.From.Set, "a text has no From address")
	assert.False(t, result.Captured)
	texts := env.Twilio.Messages()
	require.Len(t, texts, 1)
	assert.Equal(t, "+14155550199", texts[0].To, "the typed number in E.164")
	assert.Equal(t, testhelper.TwilioNumber, texts[0].From)
	assert.Equal(t, "Hi Alex, stop: ", texts[0].Body, "sample merge data, no unsubscribe link")
	assert.Empty(t, texts[0].StatusCallback, "no delivery report: there is no message for it to land on")
	messagesAfter := env.DB.OutboundMessage.Query().CountX(ctx)
	assert.Equal(t, messagesBefore, messagesAfter, "a Test send leaves no Outbound message")
	assert.IsType(t, &siteapi.SiteBroadcastsTestSendUnprocessableEntity{}, invalid, "a number without its country code is refused")
	assert.IsType(t, &siteapi.SiteBroadcastsTestSendUnprocessableEntity{}, emailOnly, "an SMS Broadcast is tested on a phone")
}

// An email Broadcast is tested on an address: a Test send naming only a phone names none.
func TestSiteEmailBroadcastTestSendWithoutAnEmailIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteBroadcastsTestSend(t.Context(), &siteapi.TestSendBroadcastInput{Phone: siteapi.NewOptString("+14155550199")},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsTestSendUnprocessableEntity{}, res)
	mails := env.CustomerMail.Messages()
	assert.Empty(t, mails)
	texts := env.Twilio.Messages()
	assert.Empty(t, texts)
}

// Twilio's refusal of a Test send is the author's to read: a typed number that reaches no handset
// is no Contact's and is not suppressed, and a refusal of the account holds nothing.
func TestSiteSMSBroadcastTestSendRefusedByTwilioSuppressesAndHoldsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	b := createSMSBroadcast(t, c, account, "Hello", nil)
	testSend := func() siteapi.SiteBroadcastsTestSendRes {
		res, err := c.SiteBroadcastsTestSend(ctx, &siteapi.TestSendBroadcastInput{Phone: siteapi.NewOptString("+14155550199")},
			siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
		require.NoError(t, err)
		return res
	}

	env.Twilio.RefuseNextSend(twiliotest.NotMobile)
	deadNumber := testSend()
	env.Twilio.RefuseNextSend(twiliotest.AccountNotActive)
	refusedAccount := testSend()
	scheduled, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))},
		siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
	require.NoError(t, err)

	refused, ok := deadNumber.(*siteapi.SiteBroadcastsTestSendUnprocessableEntity)
	require.Truef(t, ok, "got %T", deadNumber)
	assert.Contains(t, refused.Detail.Value, "not a valid mobile number (error 21614)")
	suppressed := env.DB.Suppression.Query().Where(entsuppression.Destination("+14155550199")).ExistX(ctx)
	assert.False(t, suppressed, "a typed number is not suppressed")
	assert.IsType(t, &siteapi.SiteBroadcastsTestSendUnprocessableEntity{}, refusedAccount)
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, scheduled, "the Integration is not held by a Test send")
}

// A save refuses a text longer than Twilio sends as written, as an SMS step's does: 1,600
// characters in GSM-7, 700 in UCS-2.
func TestSiteSMSBroadcastSaveRefusesATextOverTheLengthLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	b := createSMSBroadcast(t, c, account, strings.Repeat("a", 1600), nil)

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{Name: "Long", Content: testhelper.SiteSMS(strings.Repeat("a", 1601))},
		siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	fits, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Content: testhelper.SiteSMSEdit(strings.Repeat("я", 700))},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
	require.NoError(t, err)
	edited, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Content: testhelper.SiteSMSEdit(strings.Repeat("я", 701))},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
	require.NoError(t, err)

	refused, ok := created.(*siteapi.SiteBroadcastsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", created)
	assert.Contains(t, refused.Detail.Value, "1601 characters in GSM-7")
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, fits, "700 UCS-2 characters, eleven segments, go")
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, edited, "701 UCS-2 characters are too long")
}

// The length limit counts each link of a tracked text as the Workspace's short link (ADR 0083), the
// length the Workspace resource reports for the editor's counter with the limit itself, and as
// written once tracking is off: a text whose long link takes it past 1,600 characters is saved while
// its links are tracked, and turning tracking off is refused.
func TestSiteSMSBroadcastSaveCountsTrackedLinksAsShortLinks(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	spaces, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	linkLength, err := env.OutboundModule().ShortLinkLength(ctx, env.DB.Scoped(fixtures.AcmeID))
	require.NoError(t, err)
	text := strings.Repeat("a", 1400) + " https://shop.example/" + strings.Repeat("p", 200)

	b := createSMSBroadcast(t, c, account, text, nil)
	untracked := testhelper.SiteSMS(text)
	untracked.OneOf.SmsBroadcastContentInput.LinkTracking = siteapi.NewOptBool(false)
	edited, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Content: siteapi.NewOptBroadcastContentInput(untracked)},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
	require.NoError(t, err)

	acme, found := lo.Find(spaces, func(w siteapi.SiteWorkspaceResource) bool { return w.ID == fixtures.AcmePublicID })
	require.True(t, found)
	assert.EqualValues(t, linkLength, acme.ShortLinkLength)
	assert.Equal(t, siteapi.SiteSmsLengthLimit{Gsm7: 1600, Ucs2: 700}, acme.SmsMaxLength, "Twilio's limit, for the counter")
	assert.Greater(t, linkLength, len("https://x/c/")+10)
	content := testhelper.SiteSMSOf(b)
	assert.True(t, content.LinkTracking, "tracking is on by default")
	refused, ok := edited.(*siteapi.SiteBroadcastsUpdateUnprocessableEntity)
	require.Truef(t, ok, "got %T", edited)
	assert.Contains(t, refused.Detail.Value, "1622 characters in GSM-7", "as written")
}

func TestSiteSMSBroadcastScheduleRefusesABlankText(t *testing.T) {
	env := testhelper.Setup(t)
	account := env.ConnectTwilio(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	schedule := func(text string) siteapi.SiteBroadcastsScheduleRes {
		b := createSMSBroadcast(t, c, account, text, nil)
		res, err := c.SiteBroadcastsSchedule(t.Context(), &siteapi.ScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))},
			siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
		require.NoError(t, err)
		return res
	}

	blank := schedule("  ")
	assert.IsType(t, &siteapi.SiteBroadcastsScheduleUnprocessableEntity{}, blank)
	written := schedule("Hello")
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, written)
}

func TestSiteBroadcastListFiltersSMSBroadcasts(t *testing.T) {
	env := testhelper.Setup(t)
	account := env.ConnectTwilio(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	made := createSMSBroadcast(t, c, account, "Hello", nil)

	res, err := c.SiteBroadcastsList(t.Context(), siteapi.SiteBroadcastsListParams{
		WorkspaceId: fixtures.AcmePublicID, Channel: siteapi.NewOptBroadcastChannel(siteapi.BroadcastChannelSMS),
	})

	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteBroadcastsListOK)
	require.Truef(t, ok, "got %T", res)
	ids := lo.Map(list.Items, func(b siteapi.SiteBroadcastResource, _ int) siteapi.EntityId { return b.ID })
	assert.Equal(t, []siteapi.EntityId{made.ID}, ids)
}
