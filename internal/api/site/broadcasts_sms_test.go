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
	assert.Equal(t, "Hi {{ first_name }}, 20% off today", testhelper.SiteSMSOf(made).Text)
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
	assert.Equal(t, messagesBefore, env.DB.OutboundMessage.Query().CountX(ctx), "a Test send leaves no Outbound message")
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
	assert.Empty(t, env.CustomerMail.Messages())
	assert.Empty(t, env.Twilio.Messages())
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
	assert.False(t, env.DB.Suppression.Query().Where(entsuppression.Destination("+14155550199")).ExistX(ctx), "a typed number is not suppressed")
	assert.IsType(t, &siteapi.SiteBroadcastsTestSendUnprocessableEntity{}, refusedAccount)
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, scheduled, "the Integration is not held by a Test send")
}

// A save refuses a text over the part limit as written, as an SMS step's does: 10 segments of 153
// GSM-7 characters each.
func TestSiteSMSBroadcastSaveRefusesATextOverThePartLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	b := createSMSBroadcast(t, c, account, strings.Repeat("a", 1530), nil)

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{Name: "Long", Content: testhelper.SiteSMS(strings.Repeat("a", 1531))},
		siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	edited, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Content: testhelper.SiteSMSEdit(strings.Repeat("я", 671))},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
	require.NoError(t, err)

	refused, ok := created.(*siteapi.SiteBroadcastsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", created)
	assert.Contains(t, refused.Detail.Value, "11 segments")
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, edited, "671 UCS-2 characters are 11 segments of 67")
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

	assert.IsType(t, &siteapi.SiteBroadcastsScheduleUnprocessableEntity{}, schedule("  "))
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, schedule("Hello"))
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
	assert.Equal(t, []siteapi.EntityId{made.ID}, lo.Map(list.Items, func(b siteapi.SiteBroadcastResource, _ int) siteapi.EntityId { return b.ID }))
}
