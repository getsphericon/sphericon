package site_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// tokenAfter returns the token segment of a tracker URL that follows marker.
func tokenAfter(t *testing.T, url, marker string) string {
	t.Helper()
	_, token, ok := strings.Cut(url, marker)
	require.True(t, ok, "url %q lacks %s", url, marker)
	return token
}

// Double opt-in (ADR 0013): the confirmation page's button performs the
// confirmation through the site API. It is idempotent and a bad token is a 400.
func TestSitePublicConfirmationsPerform(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	anon := env.SiteAnonymous(t)

	c := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID)
	url, err := env.Tracker.ConfirmURL(tracking.ConfirmTarget{
		Destination: *c.Email, WorkspaceID: fixtures.AcmeID, ContactID: c.ID,
	})
	require.NoError(t, err)
	token := tokenAfter(t, url, "/e/confirm/")
	count := func() int {
		n, err := env.DB.Confirmation.Query().Where(
			confirmation.WorkspaceID(fixtures.AcmeID),
			confirmation.DestinationEQ(*c.Email),
		).Count(ctx)
		require.NoError(t, err)
		return n
	}
	require.Zero(t, count())

	out, err := anon.SitePublicConfirmationsPerform(ctx, siteapi.SitePublicConfirmationsPerformParams{Token: token})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicConfirmationsPerformNoContent{}, out)
	assert.Equal(t, 1, count())

	_, err = anon.SitePublicConfirmationsPerform(ctx, siteapi.SitePublicConfirmationsPerformParams{Token: token})
	require.NoError(t, err)
	assert.Equal(t, 1, count(), "repeating is a no-op")

	out, err = anon.SitePublicConfirmationsPerform(ctx, siteapi.SitePublicConfirmationsPerformParams{Token: "garbage"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicConfirmationsPerformBadRequest{}, out, "an invalid token is a 400")

	// An expired link is told apart from an invalid one (410), so the page can offer
	// to sign up again instead of a generic failure.
	expired := env.ExpiredConfirmToken(t, tracking.ConfirmTarget{
		Destination: *c.Email, WorkspaceID: fixtures.AcmeID, ContactID: c.ID,
	})
	out, err = anon.SitePublicConfirmationsPerform(ctx, siteapi.SitePublicConfirmationsPerformParams{Token: expired})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicConfirmationsPerformGone{}, out, "an expired token is a 410")
	assert.Equal(t, 1, count(), "an expired token records nothing more")
}

// Unsubscribe (ADR 0012): the page button performs the opt-out through the site
// API; the mailbox one-click POST keeps using /e/u/{token}.
func TestSitePublicUnsubscribesPerform(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	anon := env.SiteAnonymous(t)

	c := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID)
	url, err := env.Tracker.UnsubscribeURL(tracking.UnsubTarget{
		Source: eligibility.SourceBroadcasts, Destination: *c.Email, WorkspaceID: fixtures.AcmeID, ContactID: c.ID,
	})
	require.NoError(t, err)
	token := tokenAfter(t, url, "/e/u/")
	optedOut := func() bool {
		ok, err := env.DB.Unsubscribe.Query().Where(
			unsubscribe.WorkspaceID(fixtures.AcmeID),
			unsubscribe.DestinationEQ(*c.Email),
			unsubscribe.SendingSourceEQ(eligibility.SourceBroadcasts),
		).Exist(ctx)
		require.NoError(t, err)
		return ok
	}
	require.False(t, optedOut())

	out, err := anon.SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: token})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicUnsubscribesPerformNoContent{}, out)
	assert.True(t, optedOut())

	out, err = anon.SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: "garbage"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, out)
}

// ADR 0013: the confirmation event carries the confirming client's address as
// proof. Through the site operation it reaches the handler via the request
// context, so this goes over raw HTTP (an actor has no say over the proxy header).
func TestSitePublicConfirmationsPerformRecordsClientIP(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	c := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID)
	url, err := env.Tracker.ConfirmURL(tracking.ConfirmTarget{
		Destination: *c.Email, WorkspaceID: fixtures.AcmeID, ContactID: c.ID,
	})
	require.NoError(t, err)
	token := tokenAfter(t, url, "/e/confirm/")

	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/site/confirmations/"+token, nil)
	req.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.7")
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	confirmed := env.Outbox(t, "marketing.confirmed")
	require.Len(t, confirmed, 1)
	ip := confirmed[0].Data["ip"]
	assert.Equal(t, "203.0.113.7", ip)
}

// A write that really fails must not look like a success: the person would be told
// they are unsubscribed or confirmed while nothing was recorded. A NUL byte in the
// signed destination is a deterministic database failure (Postgres text rejects it)
// that is not a constraint violation.
func TestSitePublicConsentWriteFailureIsAnError(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	anon := env.SiteAnonymous(t)
	const poisoned = "a\x00b@example.com"

	unsubURL, err := env.Tracker.UnsubscribeURL(tracking.UnsubTarget{
		Source: eligibility.SourceBroadcasts, Destination: poisoned, WorkspaceID: fixtures.AcmeID,
	})
	require.NoError(t, err)
	_, err = anon.SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: tokenAfter(t, unsubURL, "/e/u/")})
	require.Error(t, err, "a failed opt-out write must not answer 204")

	confirmURL, err := env.Tracker.ConfirmURL(tracking.ConfirmTarget{Destination: poisoned, WorkspaceID: fixtures.AcmeID})
	require.NoError(t, err)
	_, err = anon.SitePublicConfirmationsPerform(ctx, siteapi.SitePublicConfirmationsPerformParams{Token: tokenAfter(t, confirmURL, "/e/confirm/")})
	require.Error(t, err, "a failed confirmation write must not answer 204")
}

// A token for a Workspace that no longer exists has nothing left to record: that is
// not a failure the person (or a retrying mailbox provider) can do anything about.
func TestSitePublicConsentForDeletedWorkspaceIsNoOp(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	anon := env.SiteAnonymous(t)
	const goneWorkspace = 999999

	unsubURL, err := env.Tracker.UnsubscribeURL(tracking.UnsubTarget{
		Source: eligibility.SourceBroadcasts, Destination: fixtures.ContactAliceEmail, WorkspaceID: goneWorkspace,
	})
	require.NoError(t, err)
	out, err := anon.SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: tokenAfter(t, unsubURL, "/e/u/")})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicUnsubscribesPerformNoContent{}, out)

	confirmURL, err := env.Tracker.ConfirmURL(tracking.ConfirmTarget{Destination: fixtures.ContactAliceEmail, WorkspaceID: goneWorkspace})
	require.NoError(t, err)
	out2, err := anon.SitePublicConfirmationsPerform(ctx, siteapi.SitePublicConfirmationsPerformParams{Token: tokenAfter(t, confirmURL, "/e/confirm/")})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicConfirmationsPerformNoContent{}, out2)
}
