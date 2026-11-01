package staff_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/trackingdomain"
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

// setTracking moves a fixture Tracking domain to state through ent, the way the recheck job does.
// The fixture rows themselves are loaded without the log's hook.
func setTracking(t *testing.T, h *harness, ws, id int64, state trackingdomain.State) {
	t.Helper()
	// test writes: the hook fires on this write
	require.NoError(t, h.env.FeedDB.Scoped(ws).TrackingDomain().UpdateOneID(id).
		SetState(state).Exec(t.Context()))
}

func TestAHostEnteringTheVerifiedStatesIsListedWithItsWorkspace(t *testing.T) {
	h := newHarness(t)

	setTracking(t, h, fixtures.AcmeID, fixtures.TrackingDomainAcmeID, trackingdomain.StateDNSVerified)

	entries, _ := hostChangesFrom(t, domainsReader(t, h), "")
	require.Len(t, entries, 1, "a pending claim of another Workspace is not listed")
	assert.Equal(t, fixtures.TrackingDomainAcmeHost, entries[0].Host)
	assert.Equal(t, serviceapi.ServiceTrackingHostStateDNSVerified, entries[0].State)
	assert.EqualValues(t, fixtures.AcmePublicID, entries[0].WorkspaceId)
}

func TestReplayingAHostFeedCursorReturnsTheSameEntries(t *testing.T) {
	h := newHarness(t)
	c := domainsReader(t, h)
	setTracking(t, h, fixtures.AcmeID, fixtures.TrackingDomainAcmeID, trackingdomain.StateDNSVerified)
	setTracking(t, h, fixtures.AcmeID, fixtures.TrackingDomainAcmeID, trackingdomain.StateServing)
	setTracking(t, h, fixtures.InitechID, fixtures.TrackingDomainDNSVerifiedID, trackingdomain.StatePending)

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

	setTracking(t, h, fixtures.InitechID, fixtures.TrackingDomainDNSVerifiedID, trackingdomain.StatePending)
	require.NoError(t, h.env.FeedDB.Scoped(fixtures.UmbrellaID).TrackingDomain().
		DeleteOneID(fixtures.TrackingDomainRetiredID).Exec(t.Context()))
	setTracking(t, h, fixtures.AcmeID, fixtures.TrackingDomainAcmeID, trackingdomain.StateServing)

	got, saved := hostChangesFrom(t, c, saved)
	assert.Equal(t, map[string]serviceapi.ServiceTrackingHostState{
		fixtures.TrackingDomainDNSVerifiedHost: serviceapi.ServiceTrackingHostStateRemoved,
		fixtures.TrackingDomainRetiredHost:     serviceapi.ServiceTrackingHostStateRemoved,
		fixtures.TrackingDomainAcmeHost:        serviceapi.ServiceTrackingHostStateServing,
	}, hostStates(got))

	again, _ := hostChangesFrom(t, c, saved)
	assert.Empty(t, again, "an unchanged host is not reported again")
}

func TestAHostMovingFromDNSVerifiedToServingIsOneEntry(t *testing.T) {
	h := newHarness(t)
	c := domainsReader(t, h)
	_, saved := hostChangesFrom(t, c, "")

	setTracking(t, h, fixtures.InitechID, fixtures.TrackingDomainDNSVerifiedID, trackingdomain.StateServing)

	got, _ := hostChangesFrom(t, c, saved)
	require.Len(t, got, 1)
	assert.Equal(t, fixtures.TrackingDomainDNSVerifiedHost, got[0].Host)
	assert.Equal(t, serviceapi.ServiceTrackingHostStateServing, got[0].State)
}

// A retired host is still served: retiring it is no change for the edge.
func TestRetiringAHostIsNotReported(t *testing.T) {
	h := newHarness(t)
	c := domainsReader(t, h)
	_, saved := hostChangesFrom(t, c, "")

	// test writes: the hook fires on this write
	require.NoError(t, h.env.FeedDB.Scoped(fixtures.UmbrellaID).TrackingDomain().
		UpdateOneID(fixtures.TrackingDomainServingID).SetRole(trackingdomain.RoleRetired).Exec(t.Context()))

	got, _ := hostChangesFrom(t, c, saved)
	assert.Empty(t, got)
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
