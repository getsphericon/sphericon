package engagement_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/engagement"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	personUA   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
	scannerUA  = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
	personAddr = "192.0.2.10" // outside every listed range
)

func openToken(t *testing.T, env *testhelper.TestEnv, messageID int64) string {
	t.Helper()
	token, err := env.Tracker.Token(env.Tracked(t, messageID))
	require.NoError(t, err)
	return token
}

func clickToken(t *testing.T, env *testhelper.TestEnv, messageID int64, dest string) string {
	t.Helper()
	link, err := env.Tracker.ClickURL(env.Tracked(t, messageID), dest)
	require.NoError(t, err)
	return link[len(env.Tracker.Origin()+"/e/c/"):]
}

func person(host int64) engagement.Hit {
	return engagement.Hit{UserAgent: personUA, Address: personAddr, Host: host, MayRecord: func() bool { return true }}
}

func TestAnOpenPublishesTheOpenOfItsMessage(t *testing.T) {
	env := testhelper.Setup(t)

	err := testhelper.Module[*engagement.Recorder](env).Open(t.Context(), openToken(t, env, fixtures.OutboundMessageTrackingSampleID), person(0))

	require.NoError(t, err)
	got := testhelper.Queued[*events.EmailEngagement](t, env, systemevent.EmailOpened)
	require.Len(t, got, 1)
	e := got[0]
	assert.Equal(t, int64(fixtures.OutboundMessageTrackingSampleID), e.OutboundMessageID)
	assert.Equal(t, int64(fixtures.AcmeID), e.WorkspaceID)
	assert.False(t, e.Machine)
}

func TestAnOpenByAScannerIsAMachineOpen(t *testing.T) {
	env := testhelper.Setup(t)
	hit := person(0)
	hit.UserAgent = scannerUA

	require.NoError(t, testhelper.Module[*engagement.Recorder](env).Open(t.Context(), openToken(t, env, fixtures.OutboundMessageTrackingSampleID), hit))

	got := testhelper.Queued[*events.EmailEngagement](t, env, systemevent.EmailOpened)
	require.Len(t, got, 1)
	e := got[0]
	assert.True(t, e.Machine)
}

// A Tracking host serves its own Workspace's links only (ADR 0038).
func TestAnOpenOnAnotherWorkspacesTrackingHostIsNotFoundAndRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	err := testhelper.Module[*engagement.Recorder](env).Open(t.Context(), openToken(t, env, fixtures.OutboundMessageTrackingSampleID), person(fixtures.GlobexID))

	require.ErrorIs(t, err, engagement.ErrNotFound)
	assert.Empty(t, env.QueuedEvents(t, systemevent.EmailOpened))
}

func TestAnOpenOnItsOwnWorkspacesTrackingHostIsRecorded(t *testing.T) {
	env := testhelper.Setup(t)

	err := testhelper.Module[*engagement.Recorder](env).Open(t.Context(), openToken(t, env, fixtures.OutboundMessageTrackingSampleID), person(fixtures.AcmeID))

	require.NoError(t, err)
	assert.Len(t, env.QueuedEvents(t, systemevent.EmailOpened), 1)
}

// Over the per-IP tracking guard a hit is answered all the same and recorded nowhere (ADR 0025),
// while a Tracking host still refuses another Workspace's link.
func TestAHitOverTheTrackingBudgetRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	hit := person(fixtures.AcmeID)
	hit.MayRecord = func() bool { return false }
	off := person(fixtures.GlobexID)
	off.MayRecord = hit.MayRecord
	token := openToken(t, env, fixtures.OutboundMessageTrackingSampleID)

	require.NoError(t, testhelper.Module[*engagement.Recorder](env).Open(t.Context(), token, hit))
	require.ErrorIs(t, testhelper.Module[*engagement.Recorder](env).Open(t.Context(), token, off), engagement.ErrNotFound)
	assert.Empty(t, env.QueuedEvents(t, systemevent.EmailOpened))
}

func TestAnOpenWithATokenThatDoesNotDecodeIsInvalid(t *testing.T) {
	env := testhelper.Setup(t)

	require.ErrorIs(t, testhelper.Module[*engagement.Recorder](env).Open(t.Context(), "not-a-token", person(0)), engagement.ErrInvalidToken)
	assert.Empty(t, env.QueuedEvents(t, systemevent.EmailOpened))
}

func TestAClickPublishesTheClickAndReturnsTheLinkItSigns(t *testing.T) {
	env := testhelper.Setup(t)
	token := clickToken(t, env, fixtures.OutboundMessageTrackingSampleID, "https://dest.test/a")

	dest, err := testhelper.Module[*engagement.Recorder](env).Click(t.Context(), token, person(0))

	require.NoError(t, err)
	assert.Equal(t, "https://dest.test/a", dest)
	got := testhelper.Queued[*events.EmailEngagement](t, env, systemevent.EmailClicked)
	require.Len(t, got, 1)
	e := got[0]
	assert.Equal(t, "https://dest.test/a", e.URL)
}

func TestAClickWithATokenThatDoesNotDecodeIsInvalid(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := testhelper.Module[*engagement.Recorder](env).Click(t.Context(), "not-a-token", person(0))

	require.ErrorIs(t, err, engagement.ErrInvalidToken)
}

func TestAShortLinkPublishesTheClickOfItsMessageAndReturnsItsURL(t *testing.T) {
	env := testhelper.Setup(t)
	token := env.DB.TrackedLink.GetX(t.Context(), fixtures.TrackedLinkAcmeID).Token

	dest, err := testhelper.Module[*engagement.Recorder](env).ShortLink(t.Context(), token, person(fixtures.AcmeID))

	require.NoError(t, err)
	assert.Equal(t, fixtures.TrackedLinkAcmeURL, dest)
	got := testhelper.Queued[*events.EmailEngagement](t, env, systemevent.EmailClicked)
	require.Len(t, got, 1, "the click of the link's email is published")
	e := got[0]
	assert.Equal(t, int64(fixtures.OutboundMessageWelcomeID), e.OutboundMessageID)
	assert.Equal(t, fixtures.TrackedLinkAcmeURL, e.URL)
}

func TestAShortLinkOnAnotherWorkspacesTrackingHostIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	token := env.DB.TrackedLink.GetX(t.Context(), fixtures.TrackedLinkAcmeID).Token

	_, err := testhelper.Module[*engagement.Recorder](env).ShortLink(t.Context(), token, person(fixtures.GlobexID))

	require.ErrorIs(t, err, engagement.ErrNotFound)
}

func TestAnUnknownShortLinkIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := testhelper.Module[*engagement.Recorder](env).ShortLink(t.Context(), "unknowntoken", person(0))

	require.ErrorIs(t, err, engagement.ErrNotFound)
}
