package operator_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/operator"
	"github.com/getsphericon/sphericon/ent/event"
	operatorapi "github.com/getsphericon/sphericon/gen/operator"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// requireMetadataOnly fails when a Workspace response carries anything but metadata
// (never Contacts, content, Events or the Workspace's secret keys).
func requireMetadataOnly(t *testing.T, w map[string]any) {
	t.Helper()
	allowed := []string{"id", "slug", "name", "createdAt", "suspension", "deliverability"}
	for k := range w {
		assert.Contains(t, allowed, k)
	}
	for _, k := range []string{"id", "slug", "name", "createdAt"} {
		assert.Contains(t, w, k)
	}
}

// operatorSession logs the enrolled fixture Operator in and returns a client bearing
// the session, and the cookie for raw requests.
func (h *harness) operatorSession(t *testing.T) (*operatorapi.Client, *http.Cookie) {
	t.Helper()
	cookie := h.session(t, fixtures.OperatorEnrolledEmail, fixtures.OperatorEnrolledPassword, fixtures.OperatorEnrolledTotpSecret)
	return h.env.OperatorWithToken(t, cookie.Value), cookie
}

func (h *harness) listWorkspaces(t *testing.T, c *operatorapi.Client, params operatorapi.OperatorWorkspacesListParams) *operatorapi.OperatorWorkspacesListOK {
	t.Helper()
	res, err := c.OperatorWorkspacesList(t.Context(), params)
	require.NoError(t, err)
	page, ok := res.(*operatorapi.OperatorWorkspacesListOK)
	require.Truef(t, ok, "got %T", res)
	return page
}

func (h *harness) getWorkspace(t *testing.T, c *operatorapi.Client, id string) operatorapi.OperatorWorkspacesGetRes {
	t.Helper()
	res, err := c.OperatorWorkspacesGet(t.Context(), operatorapi.OperatorWorkspacesGetParams{WorkspaceId: operatorapi.EntityId(id)})
	require.NoError(t, err)
	return res
}

func slugs(items []operatorapi.OperatorWorkspaceResource) []string {
	out := make([]string, 0, len(items))
	for _, w := range items {
		out = append(out, w.Slug)
	}
	return out
}

func TestTheWorkspaceListShowsEveryWorkspaceNewestFirst(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	page := h.listWorkspaces(t, c, operatorapi.OperatorWorkspacesListParams{})

	assert.Equal(t, []string{"soylent", "hooli", "umbrella", "initech", "globex", "acme"}, slugs(page.Items))
	assert.EqualValues(t, 6, page.TotalItems)
	assert.EqualValues(t, 1, page.TotalPages)
	assert.EqualValues(t, 1, page.Page)
}

func TestTheWorkspaceListIsPaginated(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	second := h.listWorkspaces(t, c, operatorapi.OperatorWorkspacesListParams{
		Page: operatorapi.NewOptInt32(2), PageSize: operatorapi.NewOptInt32(2),
	})

	assert.Equal(t, []string{"umbrella", "initech"}, slugs(second.Items))
	assert.EqualValues(t, 6, second.TotalItems)
	assert.EqualValues(t, 3, second.TotalPages)
	assert.EqualValues(t, 2, second.PageSize)
}

func TestTheWorkspaceListSearchesBySlug(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	partial := h.listWorkspaces(t, c, operatorapi.OperatorWorkspacesListParams{Slug: operatorapi.NewOptString("OOL")})
	none := h.listWorkspaces(t, c, operatorapi.OperatorWorkspacesListParams{Slug: operatorapi.NewOptString("nothing-like-this")})
	wildcard := h.listWorkspaces(t, c, operatorapi.OperatorWorkspacesListParams{Slug: operatorapi.NewOptString("%")})

	assert.Equal(t, []string{"hooli"}, slugs(partial.Items))
	assert.EqualValues(t, 1, partial.TotalItems)
	assert.Empty(t, none.Items)
	assert.EqualValues(t, 0, none.TotalItems)
	assert.Empty(t, wildcard.Items, "a wildcard character is matched literally")
}

func TestASuspendedWorkspaceShowsItsActorReasonAndTime(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	page := h.listWorkspaces(t, c, operatorapi.OperatorWorkspacesListParams{Slug: operatorapi.NewOptString("hooli")})

	require.Len(t, page.Items, 1)
	hooli := page.Items[0]
	assert.EqualValues(t, strconv.Itoa(fixtures.HooliID), hooli.ID)
	assert.Equal(t, fixtures.HooliName, hooli.Name)
	require.True(t, hooli.Suspension.Set)
	s := hooli.Suspension.Value
	// The fixture time is a wall-clock value; allow for the zone it is read in.
	assert.WithinDuration(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Time(s.At), 24*time.Hour)
	assert.Equal(t, operatorapi.OperatorSuspensionActorKindOperator, s.Actor.Kind)
	assert.Equal(t, "op-fixture", s.Actor.ID.Value, "an Operator sees the real Operator id")
	assert.Equal(t, "abuse report", s.Reason.Value)
}

func TestTheDetailShowsMetadataAndTheSuspensionState(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	acme, ok := h.getWorkspace(t, c, strconv.Itoa(fixtures.AcmeID)).(*operatorapi.OperatorWorkspaceDetailResource)
	require.True(t, ok)
	assert.Equal(t, fixtures.AcmeSlug, acme.Slug)
	assert.Equal(t, fixtures.AcmeName, acme.Name)
	assert.False(t, acme.Suspension.Set, "an active Workspace has no suspension")

	hooli, ok := h.getWorkspace(t, c, strconv.Itoa(fixtures.HooliID)).(*operatorapi.OperatorWorkspaceDetailResource)
	require.True(t, ok)
	require.True(t, hooli.Suspension.Set)
	assert.Equal(t, operatorapi.OperatorSuspensionActorKindOperator, hooli.Suspension.Value.Actor.Kind)
	assert.Equal(t, "op-fixture", hooli.Suspension.Value.Actor.ID.Value)
	assert.Equal(t, "abuse report", hooli.Suspension.Value.Reason.Value)
}

func TestAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	assert.IsType(t, &operatorapi.OperatorWorkspacesGetNotFound{}, h.getWorkspace(t, c, "999999"))
}

func TestWorkspaceResponsesCarryMetadataOnly(t *testing.T) {
	h := newHarness(t)
	_, cookie := h.operatorSession(t)

	rec := get(t, h.env.Server, "/operator/workspaces/"+strconv.Itoa(fixtures.AcmeID), cookie)
	require.Equal(t, http.StatusOK, rec.Code)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	requireMetadataOnly(t, detail)

	rec = get(t, h.env.Server, "/operator/workspaces", cookie)
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

func TestTheWorkspaceEndpointsRequireAnOperatorSession(t *testing.T) {
	h := newHarness(t)
	userToken := h.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)

	for _, path := range []string{"/operator/workspaces", "/operator/workspaces/" + strconv.Itoa(fixtures.AcmeID)} {
		assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path).Code, path)
		assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path, &http.Cookie{Name: "JWT", Value: userToken}).Code, path)
	}
}

func TestTheWorkspaceEndpointsAnswer404WithoutTheLicense(t *testing.T) {
	h := newHarness(t, testhelper.WithoutLicense())

	for _, path := range []string{"/operator/workspaces", "/operator/workspaces/" + strconv.Itoa(fixtures.AcmeID)} {
		assert.Equal(t, http.StatusNotFound, get(t, h.env.Server, path).Code, path)
	}
}

// Fixtures (events.yml): mail.soylent.test sent 8, 1 hard bounce, 1 soft bounce and 1
// complaint in the last 24 hours, plus a send 48 hours ago outside the window;
// idle.soylent.test has no traffic.
func soylentDeliverability(t *testing.T, h *harness) operatorapi.OperatorDeliverability {
	t.Helper()
	c, _ := h.operatorSession(t)
	return soylentDeliverabilityAs(t, h, c)
}

func soylentDeliverabilityAs(t *testing.T, h *harness, c *operatorapi.Client) operatorapi.OperatorDeliverability {
	t.Helper()
	soylent, ok := h.getWorkspace(t, c, strconv.Itoa(fixtures.SoylentID)).(*operatorapi.OperatorWorkspaceDetailResource)
	require.True(t, ok)
	return soylent.Deliverability
}

func domainRates(t *testing.T, d operatorapi.OperatorDeliverability, domain string) operatorapi.OperatorDomainRates {
	t.Helper()
	for _, r := range d.Domains {
		if r.Domain == domain {
			return r
		}
	}
	require.Failf(t, "domain not listed", "%s", domain)
	return operatorapi.OperatorDomainRates{}
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
	c, _ := h.operatorSession(t)
	ctx := t.Context()
	s := h.env.DB.Scoped(fixtures.SoylentID)
	inWindow := func() int {
		return s.Event().Query().Where(
			event.Action(events.NameEmailSent),
			event.OccurredAtGTE(time.Now().Add(-operator.RateWindow)),
		).CountX(ctx)
	}
	require.Equal(t, 8, inWindow(), "the fixtures hold 8 sends in the window")
	assert.EqualValues(t, inWindow(), soylentDeliverabilityAs(t, h, c).SendVolume)

	require.NoError(t, s.Event().Create().SetAction(events.NameEmailSent).SetEmail("extra@example.com").
		SetProperties(map[string]any{"sendingDomain": fixtures.SendingDomainSoylentDomain}).
		SetOccurredAt(time.Now().Add(-time.Hour)).Exec(ctx))
	require.NoError(t, s.Event().Create().SetAction(events.NameEmailSent).SetEmail("old@example.com").
		SetProperties(map[string]any{"sendingDomain": fixtures.SendingDomainSoylentDomain}).
		SetOccurredAt(time.Now().Add(-operator.RateWindow-time.Hour)).Exec(ctx))

	assert.Equal(t, 9, inWindow())
	assert.EqualValues(t, 9, soylentDeliverabilityAs(t, h, c).SendVolume, "a new send counts, one past the window does not")
}

func TestAtOrAboveTheVolumeFloorTheRatesAreDefined(t *testing.T) {
	d := soylentDeliverability(t, newHarness(t, testhelper.WithOperatorVolumeFloor(8)))

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
	c, _ := h.operatorSession(t)

	hooli, ok := h.getWorkspace(t, c, strconv.Itoa(fixtures.HooliID)).(*operatorapi.OperatorWorkspaceDetailResource)
	require.True(t, ok)
	assert.Empty(t, hooli.Deliverability.Domains)
	assert.EqualValues(t, 0, hooli.Deliverability.SendVolume)
}
