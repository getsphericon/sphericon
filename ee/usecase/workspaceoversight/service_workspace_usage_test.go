package workspaceoversight_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/usages"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func readWorkspaceUsage(t *testing.T, c *serviceapi.Client, id string, period serviceapi.OptString) *serviceapi.WorkspaceUsageResource {
	t.Helper()
	got, err := c.ServiceWorkspaceUsageGet(t.Context(), serviceapi.ServiceWorkspaceUsageGetParams{
		WorkspaceId: serviceapi.WorkspaceId(id), Period: period,
	})
	require.NoError(t, err)
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
	// The fixture's pending message holds a claim from a day ago: before the send is in flight, its
	// claim is moved out of the period.
	h.env.ClaimOutbound(t.Context(), fixtures.AcmeID, fixtures.OutboundMessageStaleClaimID, now.AddDate(0, -2, 0))
	h.env.RefreshUsage(t, now)
	first := readWorkspaceUsage(t, h.env.ServiceClient(t, primary), fixtures.AcmePublicID, serviceapi.OptString{})
	before := figureOf(t, first, serviceapi.UsageMetricEmailsSent)
	inFlightBefore := figureOf(t, first, serviceapi.UsageMetricInFlightSends)

	h.env.ClaimOutbound(t.Context(), fixtures.AcmeID, fixtures.OutboundMessageStaleClaimID, now)
	h.env.RefreshUsage(t, now)

	got := readWorkspaceUsage(t, h.env.ServiceClient(t, primary), fixtures.AcmePublicID, serviceapi.OptString{})
	assert.Equal(t, usages.PeriodOf(now).Format("2006-01"), got.Period)
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

	got := readWorkspaceUsage(t, h.env.ServiceClient(t, primary), fixtures.AcmePublicID, serviceapi.NewOptString("2025-12")) // AcmeUsageSnapshot

	assert.Equal(t, "2025-12", got.Period)
	require.NotEmpty(t, got.Figures)
	for _, f := range got.Figures {
		assert.True(t, f.Final, string(f.Metric))
	}
}

func TestReadingTheUsageOfAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := newHarness(t)

	_, err := h.env.ServiceClient(t, primary).ServiceWorkspaceUsageGet(t.Context(), serviceapi.ServiceWorkspaceUsageGetParams{WorkspaceId: "NoSuchId"})
	testhelper.ServiceRefusal(t, err, http.StatusNotFound)
}

func TestReadingOneWorkspacesUsageNeedsTheReadUsageScope(t *testing.T) {
	h := newHarness(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/workspaces/1/usage", nil)
	req.Header.Set("Authorization", "Bearer "+holdsOnly)
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
