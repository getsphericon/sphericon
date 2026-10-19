package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Globex has no verified Sending domain and Jane owns it: the Test send falls back to the
// System sender (ADR 0041).
func testSendAsJane(_ *testhelper.TestEnv, c *siteapi.Client, to string) (siteapi.SiteBroadcastsTestSendRes, error) {
	return c.SiteBroadcastsTestSend(context.Background(), &siteapi.SiteTestSendBroadcastInput{Email: siteapi.EmailAddress(to)},
		siteapi.SiteBroadcastsTestSendParams{Slug: fixtures.GlobexSlug, ID: idStr(fixtures.BroadcastGlobexID)})
}

func TestSiteTestSendWithoutVerifiedDomainGoesFromTheSystemSenderToTheAskingUser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)
	messagesBefore := env.DB.OutboundMessage.Query().CountX(t.Context())
	sentEventsBefore := env.DB.Event.Query().Where(event.Action(events.NameEmailSent)).CountX(t.Context())

	res, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)
	require.NoError(t, err)

	got, ok := res.(*siteapi.SiteTestSendBroadcastResult)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.EmailAddress(testhelper.SystemFrom), got.From, "the response names the From it used")

	sent := env.SystemMail.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, fixtures.OwnerJaneEmail, sent[0].To)
	assert.Equal(t, testhelper.SystemFrom, sent[0].From)
	assert.Empty(t, env.CustomerMail.Messages(), "no Integration is involved")
	assert.Equal(t, messagesBefore, env.DB.OutboundMessage.Query().CountX(t.Context()), "a Test send leaves no Outbound message")
	assert.Equal(t, sentEventsBefore, env.DB.Event.Query().Where(event.Action(events.NameEmailSent)).CountX(t.Context()), "and no email.sent Event")
}

func TestSiteTestSendWithoutVerifiedDomainRefusesAnyOtherRecipient(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	res, err := testSendAsJane(env, c, "stranger@example.com")
	require.NoError(t, err)

	refused, ok := res.(*siteapi.SiteBroadcastsTestSendUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Value, "your own address")
	assert.Empty(t, env.SystemMail.Messages())
	assert.Empty(t, env.CustomerMail.Messages())
}

func TestSiteTestSendWithoutVerifiedDomainMatchesTheAskersAddressCaseInsensitively(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	res, err := testSendAsJane(env, c, "JANE@Globex.test")
	require.NoError(t, err)

	assert.IsType(t, &siteapi.SiteTestSendBroadcastResult{}, res)
	assert.Len(t, env.SystemMail.Messages(), 1)
}

func TestSiteTestSendFromTheSystemSenderIsCappedPerUserPerHour(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	for i := range accounts.TestSendPerUserHour {
		res, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)
		require.NoError(t, err, "send %d", i+1)
		require.IsType(t, &siteapi.SiteTestSendBroadcastResult{}, res, "send %d", i+1)
	}

	res, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)
	require.NoError(t, err)
	limited, ok := res.(*siteapi.ProblemDetailsHeaders)
	require.Truef(t, ok, "the sixth Test send in an hour is refused, got %T", res)
	assert.Equal(t, siteapi.NewOptInt32(http.StatusTooManyRequests), limited.Response.Status)
	assert.EqualValues(t, accounts.TestSendPerUserHour, limited.XRateLimitLimit)
	assert.Positive(t, limited.RetryAfter)
	assert.Len(t, env.SystemMail.Messages(), accounts.TestSendPerUserHour, "the refused one sent nothing")
}

func TestSiteTestSendFromTheSystemSenderIsCappedPerWorkspacePerDay(t *testing.T) {
	clock := &movableClock{t: time.Now()}
	env := testhelper.Setup(t, testhelper.WithClock(clock.now))
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	// Jane spends her hourly budget four times over, an hour apart: the Workspace's twenty.
	for round := range accounts.TestSendPerWorkspaceDay / accounts.TestSendPerUserHour {
		for i := range accounts.TestSendPerUserHour {
			res, err := testSendAsJane(env, jane, fixtures.OwnerJaneEmail)
			require.NoError(t, err)
			require.IsType(t, &siteapi.SiteTestSendBroadcastResult{}, res, "round %d send %d", round, i+1)
		}
		clock.t = clock.t.Add(61 * time.Minute)
	}

	// Another User, with a budget of her own, is the Workspace's twenty-first.
	res, err := testSendAsJane(env, mary, fixtures.MemberMaryEmail)
	require.NoError(t, err)
	limited, ok := res.(*siteapi.ProblemDetailsHeaders)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.NewOptInt32(http.StatusTooManyRequests), limited.Response.Status)
	assert.EqualValues(t, accounts.TestSendPerWorkspaceDay, limited.XRateLimitLimit)
	assert.Len(t, env.SystemMail.Messages(), accounts.TestSendPerWorkspaceDay, "the refused one sent nothing")
}

func TestSiteTestSendLeavesTheSentEmailStepTodo(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := suspension.UnsuspendWorkspace(t.Context(), env.Bus, fixtures.HooliID, suspension.CLI)
	require.NoError(t, err)
	c := env.SiteActor(t, fixtures.OwnerSallyEmail)

	res, err := c.SiteBroadcastsTestSend(t.Context(), &siteapi.SiteTestSendBroadcastInput{Email: siteapi.EmailAddress(fixtures.OwnerSallyEmail)},
		siteapi.SiteBroadcastsTestSendParams{Slug: fixtures.HooliSlug, ID: idStr(fixtures.BroadcastHooliID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteTestSendBroadcastResult{}, res)
	require.Len(t, env.SystemMail.Messages(), 1)

	checklist, err := c.SiteSetupChecklistGet(t.Context(), siteapi.SiteSetupChecklistGetParams{Slug: fixtures.HooliSlug})
	require.NoError(t, err)
	progress, ok := checklist.(*siteapi.SiteSetupProgress)
	require.Truef(t, ok, "got %T", checklist)
	assert.Equal(t, todo, progress.Steps.SentEmail, "a Test send is not a sent email")
}

func TestSiteTestSendFromASuspendedWorkspaceIsRefusedWithItsReason(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)
	_, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.GlobexID, suspension.System, "abuse")
	require.NoError(t, err)

	res, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)
	require.NoError(t, err)

	refused, ok := res.(*siteapi.SiteBroadcastsTestSendUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Value, "suspended")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestSiteTestSendOnAnInstanceWithNoSystemSenderIsRefusedWithItsReason(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutSystemSender())
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	res, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)
	require.NoError(t, err)

	refused, ok := res.(*siteapi.SiteBroadcastsTestSendUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Value, "outbound mail is not configured")
}

func TestSiteTestSendFromASystemSenderWithNoFromIsRefusedNotSentWithAnEmptyOne(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithSystemSenderWithoutFrom())
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	res, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)
	require.NoError(t, err)

	refused, ok := res.(*siteapi.SiteBroadcastsTestSendUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Value, "outbound mail is not configured")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestSiteTestSendWithAVerifiedDomainKeepsTheOrdinaryPath(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteBroadcastsTestSend(t.Context(), &siteapi.SiteTestSendBroadcastInput{Email: "anyone@example.com"},
		siteapi.SiteBroadcastsTestSendParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.BroadcastDraftID)})
	require.NoError(t, err)

	got, ok := res.(*siteapi.SiteTestSendBroadcastResult)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.EmailAddress("hello@codebasics.dev"), got.From, "the Workspace's own domain")
	require.Len(t, env.CustomerMail.Messages(), 1, "through the Workspace's Integration")
	assert.Equal(t, "anyone@example.com", env.CustomerMail.Messages()[0].To, "an address the author names")
	assert.Empty(t, env.SystemMail.Messages())
}
