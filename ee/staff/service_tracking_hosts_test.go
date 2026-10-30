package staff_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/sendingdomain"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func hostChanges(t *testing.T, c *serviceapi.Client, cursor string, limit int32) *serviceapi.ServiceTrackingHostFeedPage {
	t.Helper()
	params := serviceapi.ServiceTrackingHostFeedListParams{}
	if cursor != "" {
		params.Cursor = serviceapi.NewOptString(cursor)
	}
	if limit > 0 {
		params.Limit = serviceapi.NewOptInt32(limit)
	}
	res, err := c.ServiceTrackingHostFeedList(t.Context(), params)
	require.NoError(t, err)
	page, ok := res.(*serviceapi.ServiceTrackingHostFeedPage)
	require.Truef(t, ok, "the feed is answered, got %T", res)
	return page
}

// hostChangesFrom reads every page from cursor and returns the entries and the cursor to save.
func hostChangesFrom(t *testing.T, c *serviceapi.Client, cursor string) (changes []serviceapi.ServiceTrackingHostChange, next string) {
	t.Helper()
	var all []serviceapi.ServiceTrackingHostChange
	for {
		page := hostChanges(t, c, cursor, 2)
		all = append(all, page.Items...)
		cursor = page.NextCursor
		if !page.HasMore {
			return all, cursor
		}
	}
}

// hostStates folds entries in order into each host's last reported state.
func hostStates(entries []serviceapi.ServiceTrackingHostChange) map[string]serviceapi.ServiceTrackingHostState {
	out := map[string]serviceapi.ServiceTrackingHostState{}
	for _, e := range entries {
		out[e.Host] = e.State
	}
	return out
}

func domainsReader(t *testing.T, h *harness) *serviceapi.Client {
	t.Helper()
	return h.env.ServiceClient(t, h.env.IssueServiceCredential(t, "service-edge", serviceapi.ServiceScopeDomainsRead))
}

// setTracking moves a fixture Sending domain of Umbrella to state through ent, the way the
// recheck job does. The fixture rows themselves are loaded without the log's hook.
func setTracking(t *testing.T, h *harness, id int64, state sendingdomain.TrackingState) {
	t.Helper()
	require.NoError(t, h.env.FeedDB.Scoped(fixtures.UmbrellaID).SendingDomain().UpdateOneID(id).
		SetTrackingState(state).Exec(t.Context()))
}

func TestAHostEnteringTheVerifiedStatesIsListedWithItsWorkspace(t *testing.T) {
	h := newHarness(t)

	setTracking(t, h, fixtures.SendingDomainTrackingPendingID, sendingdomain.TrackingStateDNSVerified)

	entries, _ := hostChangesFrom(t, domainsReader(t, h), "")
	require.Len(t, entries, 1, "a pending claim of another Workspace is not listed")
	assert.Equal(t, "track."+fixtures.SendingDomainTrackingPendingDomain, entries[0].Host)
	assert.Equal(t, serviceapi.ServiceTrackingHostStateDNSVerified, entries[0].State)
	assert.EqualValues(t, fixtures.UmbrellaPublicID, entries[0].WorkspaceId)
}

func TestReplayingAHostFeedCursorReturnsTheSameEntries(t *testing.T) {
	h := newHarness(t)
	c := domainsReader(t, h)
	setTracking(t, h, fixtures.SendingDomainTrackingPendingID, sendingdomain.TrackingStateDNSVerified)
	setTracking(t, h, fixtures.SendingDomainTrackingPendingID, sendingdomain.TrackingStateServing)
	setTracking(t, h, fixtures.SendingDomainTrackingDNSVerifiedID, sendingdomain.TrackingStatePending)

	first := hostChanges(t, c, "", 1)
	require.Len(t, first.Items, 1)
	require.True(t, first.HasMore)

	assert.Equal(t, first, hostChanges(t, c, "", 1))
	rest, end := hostChangesFrom(t, c, first.NextCursor)
	assert.Len(t, rest, 2)
	empty := hostChanges(t, c, end, 10)
	assert.Empty(t, empty.Items)
	assert.Equal(t, end, empty.NextCursor, "an empty page keeps the cursor")
}

func TestAHostThatLeavesTheVerifiedStatesIsReportedRemoved(t *testing.T) {
	h := newHarness(t)
	c := domainsReader(t, h)
	_, saved := hostChangesFrom(t, c, "")
	umbrella := h.env.FeedDB.Scoped(fixtures.UmbrellaID)
	dnsHost := "track." + fixtures.SendingDomainTrackingDNSVerifiedDomain
	servingHost := "track." + fixtures.SendingDomainTrackingServingDomain

	require.NoError(t, umbrella.SendingDomain().UpdateOneID(fixtures.SendingDomainTrackingDNSVerifiedID).
		SetTrackingState(sendingdomain.TrackingStatePending).Exec(t.Context()))
	require.NoError(t, umbrella.SendingDomain().PurgeOneID(fixtures.SendingDomainTrackingServingID).Exec(t.Context()))
	require.NoError(t, umbrella.SendingDomain().UpdateOneID(fixtures.SendingDomainTrackingPendingID).
		SetTrackingState(sendingdomain.TrackingStateServing).Exec(t.Context()))

	got, saved := hostChangesFrom(t, c, saved)
	assert.Equal(t, map[string]serviceapi.ServiceTrackingHostState{
		dnsHost:     serviceapi.ServiceTrackingHostStateRemoved,
		servingHost: serviceapi.ServiceTrackingHostStateRemoved,
		"track." + fixtures.SendingDomainTrackingPendingDomain: serviceapi.ServiceTrackingHostStateServing,
	}, hostStates(got))

	again, _ := hostChangesFrom(t, c, saved)
	assert.Empty(t, again, "an unchanged host is not reported again")
}

func TestAHostMovingFromDNSVerifiedToServingIsOneEntry(t *testing.T) {
	h := newHarness(t)
	c := domainsReader(t, h)
	_, saved := hostChangesFrom(t, c, "")

	require.NoError(t, h.env.FeedDB.Scoped(fixtures.UmbrellaID).SendingDomain().
		UpdateOneID(fixtures.SendingDomainTrackingDNSVerifiedID).
		SetTrackingState(sendingdomain.TrackingStateServing).Exec(t.Context()))

	got, _ := hostChangesFrom(t, c, saved)
	require.Len(t, got, 1)
	assert.Equal(t, "track."+fixtures.SendingDomainTrackingDNSVerifiedDomain, got[0].Host)
	assert.Equal(t, serviceapi.ServiceTrackingHostStateServing, got[0].State)
}

func TestARenamedSendingDomainRemovesTheOldHost(t *testing.T) {
	h := newHarness(t)
	c := domainsReader(t, h)
	_, saved := hostChangesFrom(t, c, "")

	require.NoError(t, h.env.FeedDB.Scoped(fixtures.UmbrellaID).SendingDomain().
		UpdateOneID(fixtures.SendingDomainTrackingServingID).
		SetDomain("renamed.umbrella.test").Exec(t.Context()))

	got, _ := hostChangesFrom(t, c, saved)
	assert.Equal(t, map[string]serviceapi.ServiceTrackingHostState{
		"track." + fixtures.SendingDomainTrackingServingDomain: serviceapi.ServiceTrackingHostStateRemoved,
		"track.renamed.umbrella.test":                          serviceapi.ServiceTrackingHostStateServing,
	}, hostStates(got))
}

func TestAnInvalidHostFeedCursorIsABadRequest(t *testing.T) {
	h := newHarness(t)

	res, err := domainsReader(t, h).ServiceTrackingHostFeedList(t.Context(),
		serviceapi.ServiceTrackingHostFeedListParams{Cursor: serviceapi.NewOptString("not-a-cursor!")})
	require.NoError(t, err)
	assert.IsType(t, &serviceapi.ServiceTrackingHostFeedListBadRequest{}, res)
}

func TestReadingTheHostFeedNeedsTheDomainsReadScope(t *testing.T) {
	h := newHarness(t)

	for name, value := range map[string]string{
		"workspaces and usage and holds": primary,
		"usage only":                     readOnly,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/domains/changes", nil)
		req.Header.Set("Authorization", "Bearer "+value)
		h.env.Internal.ServeHTTP(rec, req)
		assert.Equalf(t, http.StatusForbidden, rec.Code, name)
	}
	assert.NotNil(t, hostChanges(t, domainsReader(t, h), "", 1))
}

func TestWithoutALicenseTheHostFeedIsNotServed(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/domains/changes", nil)
	req.Header.Set("Authorization", "Bearer "+primary)
	env.Server.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestArchivingAHostIsReportedRemovedAndRestoringItAdded(t *testing.T) {
	h := newHarness(t)
	c := domainsReader(t, h)
	_, saved := hostChangesFrom(t, c, "")
	umbrella := h.env.FeedDB.Scoped(fixtures.UmbrellaID)
	servingHost := "track." + fixtures.SendingDomainTrackingServingDomain

	require.NoError(t, umbrella.SendingDomain().Discard(t.Context(), fixtures.SendingDomainTrackingServingID))
	got, saved := hostChangesFrom(t, c, saved)
	assert.Equal(t, map[string]serviceapi.ServiceTrackingHostState{servingHost: serviceapi.ServiceTrackingHostStateRemoved}, hostStates(got))

	require.NoError(t, umbrella.SendingDomain().Undiscard(t.Context(), fixtures.SendingDomainTrackingServingID))
	got, _ = hostChangesFrom(t, c, saved)
	assert.Equal(t, map[string]serviceapi.ServiceTrackingHostState{servingHost: serviceapi.ServiceTrackingHostStateServing}, hostStates(got))
}
