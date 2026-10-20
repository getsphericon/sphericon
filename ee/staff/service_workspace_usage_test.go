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
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func readWorkspaceUsage(t *testing.T, c *serviceapi.Client, id string, period serviceapi.OptString) *serviceapi.WorkspaceUsageResource {
	t.Helper()
	res, err := c.ServiceWorkspaceUsageGet(t.Context(), serviceapi.ServiceWorkspaceUsageGetParams{
		WorkspaceId: serviceapi.WorkspaceId(id), Period: period,
	})
	require.NoError(t, err)
	got, ok := res.(*serviceapi.WorkspaceUsageResource)
	require.Truef(t, ok, "the usage is answered, got %T", res)
	return got
}

func figureOf(t *testing.T, u *serviceapi.WorkspaceUsageResource, m serviceapi.UsageMetric) serviceapi.UsageFigure {
	t.Helper()
	for _, f := range u.Figures {
		if f.Metric == m {
			return f
		}
	}
	require.Failf(t, "no figure", "no %s figure", m)
	return serviceapi.UsageFigure{}
}

func TestAServiceReadsOneWorkspacesLiveUsageWithItsInFlightSends(t *testing.T) {
	h := newHarness(t)
	now := time.Now().UTC()
	h.env.RefreshUsage(t, now)
	first := readWorkspaceUsage(t, h.env.ServiceClient(t, primary), fixtures.AcmePublicID, serviceapi.OptString{})
	before := figureOf(t, first, serviceapi.UsageMetricEmailsSent)
	inFlightBefore := figureOf(t, first, serviceapi.UsageMetricInFlightSends)

	require.NoError(t, h.env.DB.Scoped(fixtures.AcmeID).OutboundMessage().Create().
		SetKind(outboundmessage.KindBroadcast).SetIdempotencyKey("usage-one:inflight").
		SetStatus(outboundmessage.StatusPending).SetClaimedAt(now).Exec(t.Context()))
	h.env.RefreshUsage(t, now)

	got := readWorkspaceUsage(t, h.env.ServiceClient(t, primary), fixtures.AcmePublicID, serviceapi.OptString{})
	assert.Equal(t, usage.PeriodOf(now).Format("2006-01"), got.Period)
	assert.Equal(t, serviceapi.WorkspaceId(fixtures.AcmePublicID), got.WorkspaceId)
	sent := figureOf(t, got, serviceapi.UsageMetricEmailsSent)
	assert.False(t, sent.Final)
	assert.WithinDuration(t, now, time.Time(sent.ComputedAt), time.Minute)
	assert.Equal(t, before.Value, sent.Value, "the in-flight send is not counted as sent")
	assert.Equal(t, inFlightBefore.Value+1, figureOf(t, got, serviceapi.UsageMetricInFlightSends).Value)
	assert.Len(t, got.Figures, 3, "one figure per metric, nothing about price or plan")
}

func TestAServiceReadsOneWorkspacesClosedPeriodAsFinalSnapshots(t *testing.T) {
	h := newHarness(t)
	h.env.RefreshUsage(t, time.Date(2027, time.February, 3, 0, 0, 0, 0, time.UTC))

	got := readWorkspaceUsage(t, h.env.ServiceClient(t, primary), fixtures.AcmePublicID, serviceapi.NewOptString("2027-01"))

	assert.Equal(t, "2027-01", got.Period)
	require.NotEmpty(t, got.Figures)
	for _, f := range got.Figures {
		assert.True(t, f.Final, string(f.Metric))
	}
}

func TestReadingTheUsageOfAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := newHarness(t)

	res, err := h.env.ServiceClient(t, primary).ServiceWorkspaceUsageGet(t.Context(), serviceapi.ServiceWorkspaceUsageGetParams{WorkspaceId: "NoSuchId"})
	require.NoError(t, err)
	assert.IsType(t, &serviceapi.ServiceWorkspaceUsageGetNotFound{}, res)
}

func TestReadingOneWorkspacesUsageNeedsTheReadUsageScope(t *testing.T) {
	h := newHarness(t)
	noUsage := h.env.IssueServiceCredential(t, "service-d", staff.ScopeHoldsWrite)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/workspaces/1/usage", nil)
	req.Header.Set("Authorization", "Bearer "+noUsage)
	h.env.Internal.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	assert.NotNil(t, readWorkspaceUsage(t, h.env.ServiceClient(t, readOnly), fixtures.AcmePublicID, serviceapi.OptString{}))
}

func TestWithoutALicenseTheWorkspaceUsageRouteIsNotServed(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/workspaces/1/usage", nil)
	req.Header.Set("Authorization", "Bearer "+primary)
	env.Server.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	internal := httptest.NewRecorder()
	env.Internal.ServeHTTP(internal, req)
	assert.Equal(t, http.StatusNotFound, internal.Code, "the internal listener routes no service endpoint without the license")
}
