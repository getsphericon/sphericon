package staff_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// usageOf is the figure of a metric of a Workspace in a page of the usage read.
func usageOf(t *testing.T, page *serviceapi.ServiceUsageListOK, workspaceID string, metric serviceapi.UsageMetric) serviceapi.UsageFigure {
	t.Helper()
	for _, w := range page.Items {
		if w.WorkspaceId != serviceapi.WorkspaceId(workspaceID) {
			continue
		}
		for _, f := range w.Figures {
			if f.Metric == metric {
				return f
			}
		}
	}
	require.Failf(t, "no figure", "workspace %s has no %s figure", workspaceID, metric)
	return serviceapi.UsageFigure{}
}

func readUsage(t *testing.T, c *serviceapi.Client, params serviceapi.ServiceUsageListParams) *serviceapi.ServiceUsageListOK {
	t.Helper()
	res, err := c.ServiceUsageList(t.Context(), params)
	require.NoError(t, err)
	page, ok := res.(*serviceapi.ServiceUsageListOK)
	require.Truef(t, ok, "the usage is answered, got %T", res)
	return page
}

func TestAServiceReadsLiveUsageOfTheCurrentPeriodByDefault(t *testing.T) {
	h := newHarness(t)
	now := time.Now().UTC()
	require.NoError(t, h.env.DB.Scoped(fixtures.AcmeID).OutboundMessage().Create().
		SetKind(outboundmessage.KindBroadcast).SetIdempotencyKey("usage-read:now").
		SetStatus(outboundmessage.StatusPending).SetClaimedAt(now).Exec(t.Context()))
	h.env.RefreshUsage(t, now)

	page := readUsage(t, h.env.ServiceClient(t, primary), serviceapi.ServiceUsageListParams{})

	acme := usageOf(t, page, fixtures.AcmePublicID, serviceapi.UsageMetricEmailsSent)
	assert.False(t, acme.Final)
	assert.WithinDuration(t, now, time.Time(acme.ComputedAt), time.Minute)
	assert.Positive(t, usageOf(t, page, fixtures.AcmePublicID, serviceapi.UsageMetricInFlightSends).Value, "the in-flight send is its own figure")
	assert.Equal(t, usage.PeriodOf(now).Format("2006-01"), page.Items[0].Period)
}

func TestAServiceReadsAClosedPeriodAsFinalSnapshots(t *testing.T) {
	h := newHarness(t)
	h.env.RefreshUsage(t, time.Date(2027, time.February, 3, 0, 0, 0, 0, time.UTC))

	page := readUsage(t, h.env.ServiceClient(t, primary), serviceapi.ServiceUsageListParams{
		Period: serviceapi.NewOptString("2027-01"),
	})

	require.NotEmpty(t, page.Items)
	assert.True(t, usageOf(t, page, fixtures.AcmePublicID, serviceapi.UsageMetricEmailsSent).Final)
	assert.Equal(t, "2027-01", page.Items[0].Period)
}

func TestTheUsageReadIsPageable(t *testing.T) {
	h := newHarness(t)
	h.env.RefreshUsage(t, time.Now())

	first := readUsage(t, h.env.ServiceClient(t, primary), serviceapi.ServiceUsageListParams{PageSize: serviceapi.NewOptInt32(1)})
	second := readUsage(t, h.env.ServiceClient(t, primary), serviceapi.ServiceUsageListParams{PageSize: serviceapi.NewOptInt32(1), Page: serviceapi.NewOptInt32(2)})

	require.Len(t, first.Items, 1)
	require.Len(t, second.Items, 1)
	assert.NotEqual(t, first.Items[0].WorkspaceId, second.Items[0].WorkspaceId)
	assert.Greater(t, first.TotalPages, int32(1))
}

func TestTheUsageReadNeedsTheReadUsageScope(t *testing.T) {
	h := newHarness(t)
	noUsage := h.env.IssueServiceCredential(t, "service-c", staff.ScopeHoldsWrite)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/usage", nil)
	req.Header.Set("Authorization", "Bearer "+noUsage)
	h.env.Internal.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/usage", nil)
	h.env.Internal.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	assert.NotNil(t, readUsage(t, h.env.ServiceClient(t, readOnly), serviceapi.ServiceUsageListParams{}), "a read-only credential carries the scope")
}

func TestAMalformedPeriodIsRefusedAndThePublicPortHasNoUsageRoute(t *testing.T) {
	h := newHarness(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/usage?period=2027-13", nil)
	req.Header.Set("Authorization", "Bearer "+primary)
	h.env.Internal.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/usage", nil)
	req.Header.Set("Authorization", "Bearer "+primary)
	h.env.Server.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rec.Body.String(), "figures")
}
