package staff_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/abuse"
	"github.com/getsphericon/sphericon/ent/event"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// requireMetadataOnly fails when a Workspace response carries anything but metadata
// (never Contacts, content, Events or the Workspace's secret keys).
func requireMetadataOnly(t *testing.T, w map[string]any) {
	t.Helper()
	// contract-copy: the keys of a staff Workspace response, not the automations sort order
	allowed := []string{"id", "name", "createdAt", "suspension", "deliverability", "holds", "exempt", "warned", "abuse", "discardedAt", "erasedAt", "sendCap"}
	for k := range w {
		assert.Contains(t, allowed, k)
	}
	// contract-copy: the keys every staff Workspace response carries, not a sort order
	for _, k := range []string{"id", "name", "createdAt"} {
		assert.Contains(t, w, k)
	}
}

// staffSession logs the enrolled fixture Staff member in and returns a client bearing
// the session, and the cookie for raw requests.
func (h *harness) staffSession(t *testing.T) (*staffeeapi.Client, *http.Cookie) {
	t.Helper()
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)
	return h.env.StaffEnterpriseWithToken(t, cookie.Value), cookie
}

func (h *harness) listWorkspaces(t *testing.T, c *staffeeapi.Client, params staffeeapi.StaffWorkspacesListParams) *staffeeapi.StaffWorkspacesListOK {
	t.Helper()
	page, err := c.StaffWorkspacesList(t.Context(), params)
	require.NoError(t, err)
	return page
}

func (h *harness) getWorkspace(t *testing.T, c *staffeeapi.Client, id string) (*staffeeapi.StaffWorkspaceDetailResource, error) {
	t.Helper()
	return c.StaffWorkspacesGet(t.Context(), staffeeapi.StaffWorkspacesGetParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
}

func names(items []staffeeapi.StaffWorkspaceResource) []string {
	out := make([]string, 0, len(items))
	for i := range items {
		out = append(out, items[i].Name)
	}
	return out
}

func TestTheWorkspaceListShowsEveryWorkspaceNewestFirst(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	page := h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{PageSize: staffeeapi.NewOptInt32(100)})

	anchors := []string{"Brightpath", "Cyberdyne", "Aviato", "Pied Piper", "Wayne", "Kramerica", "Vandelay", "Soylent", "Hooli", "Umbrella", "Initech", "Globex", "Acme"}
	listed := lo.Filter(names(page.Items), func(name string, _ int) bool { return slices.Contains(anchors, name) })
	assert.Equal(t, anchors, listed, "the anchor Workspaces, newest first")
	assert.EqualValues(t, len(page.Items), page.TotalItems, "one page holds every Workspace")
	assert.EqualValues(t, 1, page.TotalPages)
	assert.EqualValues(t, 1, page.Page)
}

func TestTheWorkspaceListIsPaginated(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	everyone := h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{PageSize: staffeeapi.NewOptInt32(100)})
	second := h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{
		Page: staffeeapi.NewOptInt32(2), PageSize: staffeeapi.NewOptInt32(2),
	})

	assert.Equal(t, names(everyone.Items[2:4]), names(second.Items), "the second page is the third and fourth Workspace")
	assert.Equal(t, everyone.TotalItems, second.TotalItems)
	assert.Equal(t, (everyone.TotalItems+1)/2, second.TotalPages)
	assert.EqualValues(t, 2, second.PageSize)
}

func TestTheWorkspaceListSearchesByIdOrName(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	search := func(q string) *staffeeapi.StaffWorkspacesListOK {
		return h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{Search: staffeeapi.NewOptString(q)})
	}
	partial := search("OOL")
	byID := search(fixtures.WaynePublicID)
	byIDPart := search("Ws5")
	wrongCaseID := search("hooliws5")
	none := search("nothing-like-this")
	wildcard := search("%")

	assert.Equal(t, []string{"Hooli"}, names(partial.Items), "the name matches case-insensitively")
	assert.Equal(t, []string{"Wayne"}, names(byID.Items))
	assert.Equal(t, []string{"Hooli"}, names(byIDPart.Items), "part of an id matches")
	assert.Empty(t, wrongCaseID.Items, "an id is case-sensitive")
	assert.EqualValues(t, 1, partial.TotalItems)
	assert.Empty(t, none.Items)
	assert.EqualValues(t, 0, none.TotalItems)
	assert.Empty(t, wildcard.Items, "a wildcard character is matched literally")
}

func TestASuspendedWorkspaceShowsItsActorReasonAndTime(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	page := h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{Search: staffeeapi.NewOptString("hooli")})

	require.Len(t, page.Items, 1)
	hooli := page.Items[0]
	assert.EqualValues(t, fixtures.HooliPublicID, hooli.ID)
	assert.Equal(t, fixtures.HooliName, hooli.Name)
	require.True(t, hooli.Suspension.Set)
	s := hooli.Suspension.Value
	// The fixture time is a wall-clock value; allow for the zone it is read in.
	assert.WithinDuration(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Time(s.At), 24*time.Hour)
	assert.Equal(t, staffeeapi.StaffSuspensionActorKindStaff, s.Actor.Kind)
	assert.Equal(t, "op-fixture", s.Actor.ID.Value, "a StaffMember sees the real StaffMember id")
	assert.Equal(t, "abuse report", s.Reason.Value)
}

func TestTheDetailShowsMetadataAndTheSuspensionState(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	acme, err := h.getWorkspace(t, c, fixtures.AcmePublicID)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmePublicID, acme.ID)
	assert.Equal(t, fixtures.AcmeName, acme.Name)
	assert.False(t, acme.Suspension.Set, "an active Workspace has no suspension")

	hooli, err := h.getWorkspace(t, c, fixtures.HooliPublicID)
	require.NoError(t, err)
	require.True(t, hooli.Suspension.Set)
	assert.Equal(t, staffeeapi.StaffSuspensionActorKindStaff, hooli.Suspension.Value.Actor.Kind)
	assert.Equal(t, "op-fixture", hooli.Suspension.Value.Actor.ID.Value)
	assert.Equal(t, "abuse report", hooli.Suspension.Value.Reason.Value)
}

func TestAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	_, err := h.getWorkspace(t, c, "NoSuchId")
	testhelper.StaffeeRefusal(t, err, http.StatusNotFound)
}

// An id that is not 8 base62 characters is refused by the contract as a 400 problem, before a handler runs.
func TestAMalformedWorkspaceIDIsABadRequest(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.staffSession(t)

	for _, bad := range []string{"abc", "has%20space", "waytoolongid"} {
		rec := get(t, h.env.Server, "/staff/workspaces/"+bad, cookie)
		assert.Equal(t, http.StatusBadRequest, rec.Code, bad)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json", bad)
	}
}

func TestWorkspaceResponsesCarryMetadataOnly(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.staffSession(t)

	rec := get(t, h.env.Server, "/staff/workspaces/"+fixtures.AcmePublicID, cookie)
	require.Equal(t, http.StatusOK, rec.Code)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	requireMetadataOnly(t, detail)

	rec = get(t, h.env.Server, "/staff/workspaces", cookie)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.NotEmpty(t, list.Items)
	for _, item := range list.Items {
		requireMetadataOnly(t, item)
	}
	assert.NotContains(t, rec.Body.String(), fixtures.AcmeCollectKey)
	assert.NotContains(t, rec.Body.String(), fixtures.IntegrationAcmeGenericAIngestKey)
}

func TestTheWorkspaceEndpointsRequireAnStaffSession(t *testing.T) {
	h := newHarness(t)
	userToken := h.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	for _, path := range []string{"/staff/workspaces", "/staff/workspaces/" + fixtures.AcmePublicID} {
		assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path).Code, path)
		assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path, testhelper.RequestCookie("JWT", userToken)).Code, path)
	}
}

func TestTheWorkspaceEndpointsAnswer404WithoutTheLicense(t *testing.T) {
	h := newHarness(t, testhelper.WithoutLicense())

	for _, path := range []string{"/staff/workspaces", "/staff/workspaces/" + fixtures.AcmePublicID} {
		assert.Equal(t, http.StatusNotFound, get(t, h.env.Server, path).Code, path)
	}
}

// Fixtures (events.yml): mail.soylent.test sent 8, 1 hard bounce, 1 soft bounce and 1
// complaint in the last 24 hours, plus a send 48 hours ago outside the window;
// idle.soylent.test has no traffic.
func soylentDeliverability(t *testing.T, h *harness) staffeeapi.StaffDeliverability {
	t.Helper()
	c, _ := h.staffSession(t)
	return soylentDeliverabilityAs(t, h, c)
}

func soylentDeliverabilityAs(t *testing.T, h *harness, c *staffeeapi.Client) staffeeapi.StaffDeliverability {
	t.Helper()
	soylent, err := h.getWorkspace(t, c, fixtures.SoylentPublicID)
	require.NoError(t, err)
	return soylent.Deliverability
}

func domainRates(t *testing.T, d staffeeapi.StaffDeliverability, domain string) staffeeapi.StaffDomainRates {
	t.Helper()
	for _, r := range d.Domains {
		if r.Domain == domain {
			return r
		}
	}
	require.Failf(t, "domain not listed", "%s", domain)
	return staffeeapi.StaffDomainRates{}
}

func TestBelowTheVolumeFloorTheRatesAreUndefinedButTheCountsShow(t *testing.T) {
	d := soylentDeliverability(t, newHarness(t))

	assert.EqualValues(t, 24, d.WindowHours)
	assert.EqualValues(t, 1000, d.VolumeFloor)
	assert.EqualValues(t, 8, d.SendVolume, "the send 48 hours ago is outside the window")
	require.Len(t, d.Domains, 2)

	mail := domainRates(t, d, fixtures.SendingDomainSoylentDomain)
	assert.EqualValues(t, 1, mail.ComplaintRate.Numerator)
	assert.EqualValues(t, 6, mail.ComplaintRate.Denominator, "complaints are over delivered")
	assert.True(t, mail.ComplaintRate.Rate.Null, "8 sent is below the floor")
	assert.EqualValues(t, 1, mail.BounceRate.Numerator, "a soft bounce is not counted")
	assert.EqualValues(t, 8, mail.BounceRate.Denominator)
	assert.True(t, mail.BounceRate.Rate.Null)

	idle := domainRates(t, d, fixtures.SendingDomainSoylentIdleDomain)
	assert.EqualValues(t, 0, idle.BounceRate.Denominator)
	assert.True(t, idle.BounceRate.Rate.Null)
	assert.True(t, idle.ComplaintRate.Rate.Null)
}

// The send volume is the number of email.sent Events of the Workspace in the window, read
// straight from the Events, not derived from the rates it sits beside.
func TestTheSendVolumeIsTheCountOfSentEventsInTheWindow(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	ctx := t.Context()
	s := h.env.DB.Scoped(fixtures.SoylentID)
	sent := s.Event().Query().Where(event.Action(events.NameEmailSent))
	inWindow := sent.Clone().Where(event.OccurredAtGTE(time.Now().Add(-abuse.Window))).CountX(ctx)
	require.Greater(t, sent.CountX(ctx), inWindow, "the fixtures hold a send older than the window")

	assert.EqualValues(t, inWindow, soylentDeliverabilityAs(t, h, c).SendVolume, "a send past the window does not count")
}

func TestAtOrAboveTheVolumeFloorTheRatesAreDefined(t *testing.T) {
	d := soylentDeliverability(t, newHarness(t, testhelper.WithStaffVolumeFloor(8)))

	assert.EqualValues(t, 8, d.VolumeFloor)
	mail := domainRates(t, d, fixtures.SendingDomainSoylentDomain)
	require.False(t, mail.ComplaintRate.Rate.Null)
	assert.InDelta(t, 1.0/6.0, mail.ComplaintRate.Rate.Value, 1e-9)
	require.False(t, mail.BounceRate.Rate.Null)
	assert.InDelta(t, 1.0/8.0, mail.BounceRate.Rate.Value, 1e-9)

	idle := domainRates(t, d, fixtures.SendingDomainSoylentIdleDomain)
	assert.True(t, idle.BounceRate.Rate.Null, "no traffic is below any floor")
}

func TestAWorkspaceWithoutSendingDomainsHasNoRatesAndNoVolume(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	hooli, err := h.getWorkspace(t, c, fixtures.HooliPublicID)
	require.NoError(t, err)
	assert.Empty(t, hooli.Deliverability.Domains)
	assert.EqualValues(t, 0, hooli.Deliverability.SendVolume)
}

func (h *harness) acmeHolds(t *testing.T, c *staffeeapi.Client) []staffeeapi.StaffHold {
	t.Helper()
	detail, err := h.getWorkspace(t, c, fixtures.AcmePublicID)
	require.NoError(t, err)
	return detail.Holds
}

func TestTheWorkspaceDetailListsItsActiveHolds(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.env.ExtendHold(t.Context(), fixtures.AcmeID, fixtures.HoldAcmeLapsedID, h.clock.now().Add(time.Hour))

	holds := h.acmeHolds(t, c)

	require.Len(t, holds, 1)
	assert.Equal(t, fixtures.HoldAcmeLapsedKey, holds[0].Key)
	assert.Equal(t, "review", holds[0].ReasonCode)
	assert.Equal(t, "Sending is paused for now.", holds[0].Message)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, holds[0].SetBy)
	assert.WithinDuration(t, h.clock.now().Add(time.Hour), time.Time(holds[0].ExpiresAt), time.Second)
	stored := h.env.DB.Hold.GetX(t.Context(), fixtures.HoldAcmeLapsedID)
	assert.WithinDuration(t, stored.CreatedAt, time.Time(holds[0].CreatedAt), time.Second)
}

func TestAnExpiredHoldIsNotListedAndAnUnaffectedWorkspaceShowsNone(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.env.ExtendHold(t.Context(), fixtures.AcmeID, fixtures.HoldAcmeLapsedID, h.clock.now().Add(time.Hour))
	h.clock.t = h.clock.t.Add(2 * time.Hour)

	assert.Empty(t, h.acmeHolds(t, c), "the fixture's lapsed holds and the expired one are not active")
	globex, err := h.getWorkspace(t, c, fixtures.GlobexPublicID)
	require.NoError(t, err)
	assert.Empty(t, globex.Holds)
}

func TestTheWorkspaceListHidesADeletedWorkspaceUnlessFilteredToDeleted(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	discardedAt := h.env.DB.Workspace.GetX(t.Context(), fixtures.BluthID).DiscardedAt
	require.NotNil(t, discardedAt)

	searched := h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{Search: staffeeapi.NewOptString(fixtures.BluthPublicID)})
	deleted := h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{Review: staffeeapi.NewOptStaffWorkspaceReview(staffeeapi.StaffWorkspaceReviewDeleted)})

	assert.Empty(t, searched.Items, "the default list hides Bluth even when searched for")
	require.Equal(t, []string{fixtures.BluthName}, names(deleted.Items))
	got, ok := deleted.Items[0].DiscardedAt.Get()
	require.True(t, ok)
	assert.WithinDuration(t, *discardedAt, time.Time(got), time.Second)
}
