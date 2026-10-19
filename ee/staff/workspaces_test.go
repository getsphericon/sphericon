package staff_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ent/event"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// requireMetadataOnly fails when a Workspace response carries anything but metadata
// (never Contacts, content, Events or the Workspace's secret keys).
func requireMetadataOnly(t *testing.T, w map[string]any) {
	t.Helper()
	allowed := []string{"id", "slug", "name", "createdAt", "suspension", "deliverability", "holds", "exempt", "warned", "abuse"}
	for k := range w {
		assert.Contains(t, allowed, k)
	}
	for _, k := range []string{"id", "slug", "name", "createdAt"} {
		assert.Contains(t, w, k)
	}
}

// staffSession logs the enrolled fixture Staff member in and returns a client bearing
// the session, and the cookie for raw requests.
func (h *harness) staffSession(t *testing.T) (*staffapi.Client, *http.Cookie) {
	t.Helper()
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)
	return h.env.StaffWithToken(t, cookie.Value), cookie
}

func (h *harness) listWorkspaces(t *testing.T, c *staffapi.Client, params staffapi.StaffWorkspacesListParams) *staffapi.StaffWorkspacesListOK {
	t.Helper()
	res, err := c.StaffWorkspacesList(t.Context(), params)
	require.NoError(t, err)
	page, ok := res.(*staffapi.StaffWorkspacesListOK)
	require.Truef(t, ok, "got %T", res)
	return page
}

func (h *harness) getWorkspace(t *testing.T, c *staffapi.Client, id int64) staffapi.StaffWorkspacesGetRes {
	t.Helper()
	res, err := c.StaffWorkspacesGet(t.Context(), staffapi.StaffWorkspacesGetParams{WorkspaceId: staffapi.EntityId(id)})
	require.NoError(t, err)
	return res
}

func slugs(items []staffapi.StaffWorkspaceResource) []string {
	out := make([]string, 0, len(items))
	for _, w := range items {
		out = append(out, w.Slug)
	}
	return out
}

func TestTheWorkspaceListShowsEveryWorkspaceNewestFirst(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	page := h.listWorkspaces(t, c, staffapi.StaffWorkspacesListParams{})

	assert.Equal(t, []string{"wayne", "kramerica", "vandelay", "soylent", "hooli", "umbrella", "initech", "globex", "acme"}, slugs(page.Items))
	assert.EqualValues(t, 9, page.TotalItems)
	assert.EqualValues(t, 1, page.TotalPages)
	assert.EqualValues(t, 1, page.Page)
}

func TestTheWorkspaceListIsPaginated(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	second := h.listWorkspaces(t, c, staffapi.StaffWorkspacesListParams{
		Page: staffapi.NewOptInt32(2), PageSize: staffapi.NewOptInt32(2),
	})

	assert.Equal(t, []string{"vandelay", "soylent"}, slugs(second.Items))
	assert.EqualValues(t, 9, second.TotalItems)
	assert.EqualValues(t, 5, second.TotalPages)
	assert.EqualValues(t, 2, second.PageSize)
}

func TestTheWorkspaceListSearchesBySlug(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	partial := h.listWorkspaces(t, c, staffapi.StaffWorkspacesListParams{Slug: staffapi.NewOptString("OOL")})
	none := h.listWorkspaces(t, c, staffapi.StaffWorkspacesListParams{Slug: staffapi.NewOptString("nothing-like-this")})
	wildcard := h.listWorkspaces(t, c, staffapi.StaffWorkspacesListParams{Slug: staffapi.NewOptString("%")})

	assert.Equal(t, []string{"hooli"}, slugs(partial.Items))
	assert.EqualValues(t, 1, partial.TotalItems)
	assert.Empty(t, none.Items)
	assert.EqualValues(t, 0, none.TotalItems)
	assert.Empty(t, wildcard.Items, "a wildcard character is matched literally")
}

func TestASuspendedWorkspaceShowsItsActorReasonAndTime(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	page := h.listWorkspaces(t, c, staffapi.StaffWorkspacesListParams{Slug: staffapi.NewOptString("hooli")})

	require.Len(t, page.Items, 1)
	hooli := page.Items[0]
	assert.EqualValues(t, fixtures.HooliID, hooli.ID)
	assert.Equal(t, fixtures.HooliName, hooli.Name)
	require.True(t, hooli.Suspension.Set)
	s := hooli.Suspension.Value
	// The fixture time is a wall-clock value; allow for the zone it is read in.
	assert.WithinDuration(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Time(s.At), 24*time.Hour)
	assert.Equal(t, staffapi.StaffSuspensionActorKindStaff, s.Actor.Kind)
	assert.Equal(t, "op-fixture", s.Actor.ID.Value, "a StaffMember sees the real StaffMember id")
	assert.Equal(t, "abuse report", s.Reason.Value)
}

func TestTheDetailShowsMetadataAndTheSuspensionState(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	acme, ok := h.getWorkspace(t, c, int64(fixtures.AcmeID)).(*staffapi.StaffWorkspaceDetailResource)
	require.True(t, ok)
	assert.Equal(t, fixtures.AcmeSlug, acme.Slug)
	assert.Equal(t, fixtures.AcmeName, acme.Name)
	assert.False(t, acme.Suspension.Set, "an active Workspace has no suspension")

	hooli, ok := h.getWorkspace(t, c, int64(fixtures.HooliID)).(*staffapi.StaffWorkspaceDetailResource)
	require.True(t, ok)
	require.True(t, hooli.Suspension.Set)
	assert.Equal(t, staffapi.StaffSuspensionActorKindStaff, hooli.Suspension.Value.Actor.Kind)
	assert.Equal(t, "op-fixture", hooli.Suspension.Value.Actor.ID.Value)
	assert.Equal(t, "abuse report", hooli.Suspension.Value.Reason.Value)
}

func TestAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	assert.IsType(t, &staffapi.StaffWorkspacesGetNotFound{}, h.getWorkspace(t, c, 999999))
}

// An id that is not an int64 is refused by the contract as a 400 problem, before a handler runs.
func TestAMalformedWorkspaceIDIsABadRequest(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.staffSession(t)

	for _, bad := range []string{"abc", "99999999999999999999"} {
		rec := get(t, h.env.Server, "/staff/workspaces/"+bad, cookie)
		assert.Equal(t, http.StatusBadRequest, rec.Code, bad)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json", bad)
	}
}

func TestWorkspaceResponsesCarryMetadataOnly(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.staffSession(t)

	rec := get(t, h.env.Server, "/staff/workspaces/"+strconv.Itoa(fixtures.AcmeID), cookie)
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
	assert.NotContains(t, rec.Body.String(), fixtures.AcmeIngestKey)
}

func TestTheWorkspaceEndpointsRequireAnStaffSession(t *testing.T) {
	h := newHarness(t)
	userToken := h.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	for _, path := range []string{"/staff/workspaces", "/staff/workspaces/" + strconv.Itoa(fixtures.AcmeID)} {
		assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path).Code, path)
		assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path, &http.Cookie{Name: "JWT", Value: userToken}).Code, path)
	}
}

func TestTheWorkspaceEndpointsAnswer404WithoutTheLicense(t *testing.T) {
	h := newHarness(t, testhelper.WithoutLicense())

	for _, path := range []string{"/staff/workspaces", "/staff/workspaces/" + strconv.Itoa(fixtures.AcmeID)} {
		assert.Equal(t, http.StatusNotFound, get(t, h.env.Server, path).Code, path)
	}
}

// Fixtures (events.yml): mail.soylent.test sent 8, 1 hard bounce, 1 soft bounce and 1
// complaint in the last 24 hours, plus a send 48 hours ago outside the window;
// idle.soylent.test has no traffic.
func soylentDeliverability(t *testing.T, h *harness) staffapi.StaffDeliverability {
	t.Helper()
	c, _ := h.staffSession(t)
	return soylentDeliverabilityAs(t, h, c)
}

func soylentDeliverabilityAs(t *testing.T, h *harness, c *staffapi.Client) staffapi.StaffDeliverability {
	t.Helper()
	soylent, ok := h.getWorkspace(t, c, int64(fixtures.SoylentID)).(*staffapi.StaffWorkspaceDetailResource)
	require.True(t, ok)
	return soylent.Deliverability
}

func domainRates(t *testing.T, d staffapi.StaffDeliverability, domain string) staffapi.StaffDomainRates {
	t.Helper()
	for _, r := range d.Domains {
		if r.Domain == domain {
			return r
		}
	}
	require.Failf(t, "domain not listed", "%s", domain)
	return staffapi.StaffDomainRates{}
}

func TestBelowTheVolumeFloorTheRatesAreUndefinedButTheCountsShow(t *testing.T) {
	d := soylentDeliverability(t, newHarness(t))

	assert.EqualValues(t, 24, d.WindowHours)
	assert.EqualValues(t, 1000, d.VolumeFloor)
	assert.EqualValues(t, 8, d.SendVolume, "the send 48 hours ago is outside the window")
	require.Len(t, d.Domains, 2)

	mail := domainRates(t, d, fixtures.SendingDomainSoylentDomain)
	assert.EqualValues(t, 1, mail.ComplaintRate.Numerator)
	assert.EqualValues(t, 7, mail.ComplaintRate.Denominator, "complaints are over sent minus hard bounces")
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
	inWindow := func() int {
		return s.Event().Query().Where(
			event.Action(events.NameEmailSent),
			event.OccurredAtGTE(time.Now().Add(-abuse.Window)),
		).CountX(ctx)
	}
	require.Equal(t, 8, inWindow(), "the fixtures hold 8 sends in the window")
	assert.EqualValues(t, inWindow(), soylentDeliverabilityAs(t, h, c).SendVolume)

	require.NoError(t, s.Event().Create().SetAction(events.NameEmailSent).SetEmail("extra@example.com").
		SetProperties(map[string]any{"sendingDomain": fixtures.SendingDomainSoylentDomain}).
		SetOccurredAt(time.Now().Add(-time.Hour)).Exec(ctx))
	require.NoError(t, s.Event().Create().SetAction(events.NameEmailSent).SetEmail("old@example.com").
		SetProperties(map[string]any{"sendingDomain": fixtures.SendingDomainSoylentDomain}).
		SetOccurredAt(time.Now().Add(-abuse.Window-time.Hour)).Exec(ctx))

	assert.Equal(t, 9, inWindow())
	assert.EqualValues(t, 9, soylentDeliverabilityAs(t, h, c).SendVolume, "a new send counts, one past the window does not")
}

func TestAtOrAboveTheVolumeFloorTheRatesAreDefined(t *testing.T) {
	d := soylentDeliverability(t, newHarness(t, testhelper.WithStaffVolumeFloor(8)))

	assert.EqualValues(t, 8, d.VolumeFloor)
	mail := domainRates(t, d, fixtures.SendingDomainSoylentDomain)
	require.False(t, mail.ComplaintRate.Rate.Null)
	assert.InDelta(t, 1.0/7.0, mail.ComplaintRate.Rate.Value, 1e-9)
	require.False(t, mail.BounceRate.Rate.Null)
	assert.InDelta(t, 1.0/8.0, mail.BounceRate.Rate.Value, 1e-9)

	idle := domainRates(t, d, fixtures.SendingDomainSoylentIdleDomain)
	assert.True(t, idle.BounceRate.Rate.Null, "no traffic is below any floor")
}

func TestAWorkspaceWithoutSendingDomainsHasNoRatesAndNoVolume(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	hooli, ok := h.getWorkspace(t, c, int64(fixtures.HooliID)).(*staffapi.StaffWorkspaceDetailResource)
	require.True(t, ok)
	assert.Empty(t, hooli.Deliverability.Domains)
	assert.EqualValues(t, 0, hooli.Deliverability.SendVolume)
}

func (h *harness) acmeHolds(t *testing.T, c *staffapi.Client) []staffapi.StaffHold {
	t.Helper()
	detail, ok := h.getWorkspace(t, c, int64(fixtures.AcmeID)).(*staffapi.StaffWorkspaceDetailResource)
	require.True(t, ok)
	return detail.Holds
}

func TestTheWorkspaceDetailListsItsActiveHolds(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.setHold(t, "k1", time.Hour)

	holds := h.acmeHolds(t, c)

	require.Len(t, holds, 1)
	assert.Equal(t, "k1", holds[0].Key)
	assert.Equal(t, "review", holds[0].ReasonCode)
	assert.Equal(t, "Sending is paused for now.", holds[0].Message)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, holds[0].SetBy)
	assert.WithinDuration(t, h.clock.now().Add(time.Hour), time.Time(holds[0].ExpiresAt), time.Second)
	assert.WithinDuration(t, h.clock.now(), time.Time(holds[0].CreatedAt), time.Minute)
}

func TestAnExpiredHoldIsNotListedAndAnUnaffectedWorkspaceShowsNone(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.setHold(t, "k1", time.Hour)
	h.clock.t = h.clock.t.Add(2 * time.Hour)

	assert.Empty(t, h.acmeHolds(t, c), "the fixture's lapsed hold and the expired one are not active")
	globex, ok := h.getWorkspace(t, c, int64(fixtures.GlobexID)).(*staffapi.StaffWorkspaceDetailResource)
	require.True(t, ok)
	assert.Empty(t, globex.Holds)
}
