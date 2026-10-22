package server_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/telemetry"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tracking guard only decides whether an engagement is recorded (ADR 0025):
// a recipient is never refused, whatever the request rate of their IP.

const clickDest = "https://dest.test/x"

func withTrackingLimit(limit int) testhelper.Option {
	return testhelper.WithRateLimits(config.RateLimits{Tracking: limit})
}

func trackingGet(t *testing.T, env *testhelper.TestEnv, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func trackingToken(t *testing.T, env *testhelper.TestEnv, messageID int64) string {
	t.Helper()
	token, err := env.Tracker.Token(messageID)
	require.NoError(t, err)
	return token
}

func clickPath(t *testing.T, env *testhelper.TestEnv, messageID int64) string {
	t.Helper()
	link, err := env.Tracker.ClickURL(messageID, clickDest)
	require.NoError(t, err)
	_, token, _ := strings.Cut(link, "/e/c/")
	return "/e/c/" + token
}

func TestAClickRedirectsOnlyToTheDestinationItsTokenSigns(t *testing.T) {
	env := testhelper.Setup(t)
	recipient := trackingToken(t, env, fixtures.OutboundMessageUnengagedFirstID)

	// The recipient token also rides the open pixel: with a destination of the caller's
	// choosing it is no click link.
	rec := trackingGet(t, env, "/e/c/"+recipient+"?u="+url.QueryEscape("https://evil.test/"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, rec.Header().Get("Location"))

	// A parameter next to a click token does not replace the signed destination.
	signed := clickPath(t, env, fixtures.OutboundMessageUnengagedFirstID)
	rec = trackingGet(t, env, signed+"?u="+url.QueryEscape("https://evil.test/"))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, clickDest, rec.Header().Get("Location"))
}

func TestOverTheTrackingGuardAClickStillRedirectsButIsNotRecorded(t *testing.T) {
	env := testhelper.Setup(t, withTrackingLimit(1))

	first := trackingGet(t, env, clickPath(t, env, fixtures.OutboundMessageUnengagedFirstID))
	require.Equal(t, http.StatusFound, first.Code)
	rec := trackingGet(t, env, clickPath(t, env, fixtures.OutboundMessageUnengagedSecondID))

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, clickDest, rec.Header().Get("Location"))
	assert.NotNil(t, env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientUnengagedFirstID).ClickedAt)
	assert.Nil(t, env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientUnengagedSecondID).ClickedAt)
}

func TestOverTheTrackingGuardAnOpenStillReturnsThePixelButIsNotRecorded(t *testing.T) {
	env := testhelper.Setup(t, withTrackingLimit(1))
	first := trackingToken(t, env, fixtures.OutboundMessageUnengagedFirstID)
	second := trackingToken(t, env, fixtures.OutboundMessageUnengagedSecondID)

	require.Equal(t, http.StatusOK, trackingGet(t, env, "/e/o/"+first).Code)
	rec := trackingGet(t, env, "/e/o/"+second)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "image/gif", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Body.Bytes())
	assert.NotNil(t, env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientUnengagedFirstID).OpenedAt)
	assert.Nil(t, env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientUnengagedSecondID).OpenedAt)
}

func TestBelowTheTrackingGuardEngagementsAreRecorded(t *testing.T) {
	env := testhelper.Setup(t, withTrackingLimit(5))
	first := trackingToken(t, env, fixtures.OutboundMessageUnengagedFirstID)

	opened := trackingGet(t, env, "/e/o/"+first)
	require.Equal(t, http.StatusOK, opened.Code)
	clicked := trackingGet(t, env, clickPath(t, env, fixtures.OutboundMessageUnengagedSecondID))
	require.Equal(t, http.StatusFound, clicked.Code)

	assert.NotNil(t, env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientUnengagedFirstID).OpenedAt)
	assert.NotNil(t, env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientUnengagedSecondID).ClickedAt)
}

func TestOneClickUnsubscribeIsNeverRateLimited(t *testing.T) {
	env := testhelper.Setup(t, withTrackingLimit(1))
	path := unsubPath(t, env.Tracker, tracking.UnsubTarget{
		Source: eligibility.SourceBroadcasts, Destination: fixtures.ContactAliceEmail,
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID,
	})
	for i := range 10 {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		env.Server.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code, "POST %d", i+1)
		require.Equal(t, http.StatusSeeOther, trackingGet(t, env, path).Code, "GET %d", i+1)
	}
}

func TestASkippedRecordingIsCountedUnderTheTrackingPolicy(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	testhelper.InstallOtel(t, func(ctx context.Context) (func(context.Context) error, error) {
		return telemetry.Setup(ctx, &config.Config{OtelServiceName: "sphericon-test"}, "test", telemetry.BuildInfo{})
	})

	env := testhelper.Setup(t, withTrackingLimit(1))
	first := trackingToken(t, env, fixtures.OutboundMessageUnengagedFirstID)
	second := trackingToken(t, env, fixtures.OutboundMessageUnengagedSecondID)
	trackingGet(t, env, "/e/o/"+first)
	trackingGet(t, env, "/e/o/"+second)

	scrape := httptest.NewRecorder()
	telemetry.MetricsHandler().ServeHTTP(scrape, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	assert.Regexp(t, `ratelimit_rejected_total\{[^}]*policy="tracking"[^}]*\} 1`, scrape.Body.String())
	assert.Contains(t, logs.String(), `"policy":"tracking"`)
}

func TestAZeroTrackingLimitRecordsEverything(t *testing.T) {
	env := testhelper.Setup(t, withTrackingLimit(0))
	for message, recipient := range map[int64]int64{
		fixtures.OutboundMessageUnengagedFirstID:  fixtures.BroadcastRecipientUnengagedFirstID,
		fixtures.OutboundMessageUnengagedSecondID: fixtures.BroadcastRecipientUnengagedSecondID,
	} {
		require.Equal(t, http.StatusOK, trackingGet(t, env, "/e/o/"+trackingToken(t, env, message)).Code)
		assert.NotNil(t, env.DB.BroadcastRecipient.GetX(t.Context(), recipient).OpenedAt)
	}
}
