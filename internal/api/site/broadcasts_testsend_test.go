package site_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Globex has no verified Sending domain and Jane owns it: the Test send falls back to the
// System sender (ADR 0041).
func testSendAsJane(_ *testhelper.TestEnv, c *siteapi.Client, to string) (*siteapi.SiteTestSendBroadcastResult, error) {
	return c.SiteBroadcastsTestSend(context.Background(), &siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress(siteapi.EmailAddress(to))},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.BroadcastGlobexID)})
}

// testSendOverHTTP is testSendAsJane's request sent by the User of email, answered whole: the
// generated client keeps no header of a refusal, and a rate limit is told in its headers.
func testSendOverHTTP(t *testing.T, env *testhelper.TestEnv, email, to string) *httptest.ResponseRecorder {
	t.Helper()
	path := fmt.Sprintf("/site/workspaces/%s/broadcasts/%d/test-send", fixtures.GlobexPublicID, fixtures.BroadcastGlobexID)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(`{"email":"`+to+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testhelper.RequestCookie("JWT", env.SiteToken(t, email, nil)))
	env.Server.ServeHTTP(rec, req)
	return rec
}

func TestSiteTestSendWithoutVerifiedDomainGoesFromTheSystemSenderToTheAskingUser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)
	messagesBefore := env.DB.OutboundMessage.Query().CountX(t.Context())
	sentEventsBefore := env.DB.Event.Query().Where(event.Action(events.NameEmailSent)).CountX(t.Context())

	res, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)
	require.NoError(t, err)

	got := res
	assert.Equal(t, siteapi.NewOptEmailAddress(siteapi.EmailAddress(testhelper.SystemFrom)), got.From, "the response names the From it used")

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

	_, err := testSendAsJane(env, c, "stranger@example.com")

	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "your own address")
	assert.Empty(t, env.SystemMail.Messages())
	assert.Empty(t, env.CustomerMail.Messages())
}

func TestSiteTestSendWithoutVerifiedDomainMatchesTheAskersAddressCaseInsensitively(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	_, err := testSendAsJane(env, c, "JANE@Globex.test")
	require.NoError(t, err)

	assert.Len(t, env.SystemMail.Messages(), 1)
}

func TestSiteTestSendFromTheSystemSenderIsCappedPerUserPerHour(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	for i := range throttle.TestSendPerUserHour {
		_, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)
		require.NoError(t, err, "send %d", i+1)
	}

	limited := testSendOverHTTP(t, env, fixtures.OwnerJaneEmail, fixtures.OwnerJaneEmail)
	require.Equal(t, http.StatusTooManyRequests, limited.Code, "the sixth Test send in an hour is refused")
	var problem siteapi.ProblemDetails
	require.NoError(t, problem.UnmarshalJSON(limited.Body.Bytes()))
	retryAfter, err := strconv.Atoi(limited.Header().Get("Retry-After"))
	require.NoError(t, err)
	assert.Equal(t, siteapi.NewOptInt32(http.StatusTooManyRequests), problem.Status)
	assert.Equal(t, strconv.Itoa(throttle.TestSendPerUserHour), limited.Header().Get("X-Ratelimit-Limit"))
	assert.Positive(t, retryAfter)
	assert.Len(t, env.SystemMail.Messages(), throttle.TestSendPerUserHour, "the refused one sent nothing")
}

func TestSiteTestSendFromTheSystemSenderIsCappedPerWorkspacePerDay(t *testing.T) {
	clock := &movableClock{t: time.Now()}
	env := testhelper.Setup(t, testhelper.WithClock(clock.now))
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)

	// Jane spends her hourly budget four times over, an hour apart: the Workspace's twenty.
	for round := range throttle.TestSendPerWorkspaceDay / throttle.TestSendPerUserHour {
		for i := range throttle.TestSendPerUserHour {
			_, err := testSendAsJane(env, jane, fixtures.OwnerJaneEmail)
			require.NoError(t, err, "round %d send %d", round, i+1)
		}
		clock.t = clock.t.Add(61 * time.Minute)
	}

	// Another User, with a budget of her own, is the Workspace's twenty-first.
	limited := testSendOverHTTP(t, env, fixtures.MemberMaryEmail, fixtures.MemberMaryEmail)
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	var problem siteapi.ProblemDetails
	require.NoError(t, problem.UnmarshalJSON(limited.Body.Bytes()))
	assert.Equal(t, siteapi.NewOptInt32(http.StatusTooManyRequests), problem.Status)
	assert.Equal(t, strconv.Itoa(throttle.TestSendPerWorkspaceDay), limited.Header().Get("X-Ratelimit-Limit"))
	assert.Len(t, env.SystemMail.Messages(), throttle.TestSendPerWorkspaceDay, "the refused one sent nothing")
}

func TestSiteTestSendLeavesTheSentEmailStepTodo(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := suspension.UnsuspendWorkspace(t.Context(), env.Bus, fixtures.HooliID, suspension.CLI, time.Now())
	require.NoError(t, err)
	c := env.SiteActor(t, fixtures.OwnerSallyEmail)

	_, err = c.SiteBroadcastsTestSend(t.Context(), &siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress(siteapi.EmailAddress(fixtures.OwnerSallyEmail))},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.HooliPublicID, ID: idStr(fixtures.BroadcastHooliID)})
	require.NoError(t, err)

	require.Len(t, env.SystemMail.Messages(), 1)

	checklist, err := c.SiteSetupChecklistGet(t.Context(), siteapi.SiteSetupChecklistGetParams{WorkspaceId: fixtures.HooliPublicID})
	require.NoError(t, err)
	progress := checklist
	assert.Equal(t, todo, progress.Steps.SentEmail, "a Test send is not a sent email")
}

func TestSiteTestSendFromASuspendedWorkspaceIsRefusedWithItsReason(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)
	_, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.GlobexID, suspension.System, "abuse", time.Now())
	require.NoError(t, err)

	_, err = testSendAsJane(env, c, fixtures.OwnerJaneEmail)

	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "suspended")
	tests := lo.Filter(env.SystemMail.Messages(), func(m messaging.EmailMessage, _ int) bool { return !strings.Contains(m.Subject, "suspended") })
	assert.Empty(t, tests, "only the suspension notice to the owner went out, no test send")
}

func TestSiteTestSendOnAnInstanceWithNoSystemSenderIsRefusedWithItsReason(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutSystemSender())
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	_, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)

	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "outbound mail is not configured")
}

func TestSiteTestSendFromASystemSenderWithNoFromIsRefusedNotSentWithAnEmptyOne(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithSystemSenderWithoutFrom())
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)

	_, err := testSendAsJane(env, c, fixtures.OwnerJaneEmail)

	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "outbound mail is not configured")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestSiteTestSendWithAVerifiedDomainKeepsTheOrdinaryPath(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteBroadcastsTestSend(t.Context(), &siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress("anyone@example.com")},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})
	require.NoError(t, err)

	got := res
	assert.Equal(t, siteapi.NewOptEmailAddress(siteapi.EmailAddress("hello@codebasics.dev")), got.From, "the Workspace's own domain")
	require.Len(t, env.CustomerMail.Messages(), 1, "through the Workspace's Integration")
	assert.Equal(t, "anyone@example.com", env.CustomerMail.Messages()[0].To, "an address the author names")
	assert.Empty(t, env.SystemMail.Messages())
}

func TestSiteTestSendOfABroadcastOnATestInboxIsCapturedEvenToAnotherAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJaneEmail)
	env.DB.Broadcast.UpdateOneID(fixtures.BroadcastGlobexID).SetIntegrationID(fixtures.IntegrationGlobexInboxID).ExecX(t.Context())

	res, err := testSendAsJane(env, c, "stranger@example.com")
	require.NoError(t, err)

	got := res
	assert.True(t, got.Captured, "the Broadcast's own Test inbox keeps it")
	assert.Empty(t, env.SystemMail.Messages(), "not the System sender")
}
