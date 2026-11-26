package mcpserver_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An agent reads since when, why and until when an Enrollment's send waits at its step (ADR 0178;
// fixture Order series: Alice's order A, put in the wait a Send cap's Deferral records).
func TestMCPEnrollmentsShowTheWaitOfTheirSend(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeEnrollmentsRead))
	since := time.Date(2030, time.March, 4, 10, 0, 0, 0, time.UTC)
	env.DB.AutomationRun.UpdateOneID(fixtures.AutomationRunOrderSeriesAID).
		SetDeferredSince(since).SetDeferralCause(deferralcause.SendCap).SetResumeAt(since.Add(10 * time.Minute)).
		ExecX(t.Context())

	res := call(t, s, "enrollments_get", map[string]any{
		"automationId": testhelper.ID(fixtures.AutomationOrderSeriesID), "enrollmentId": testhelper.ID(fixtures.AutomationRunOrderSeriesAID),
	})

	require.False(t, res.IsError, text(t, res))
	enrollment := decode(t, text(t, res))
	deferredSince, err := time.Parse(time.RFC3339, enrollment["deferredSince"].(string))
	require.NoError(t, err)
	resumeAt, err := time.Parse(time.RFC3339, enrollment["resumeAt"].(string))
	require.NoError(t, err)
	assert.True(t, since.Equal(deferredSince), "since when")
	assert.Equal(t, "send_cap", enrollment["deferralCause"], "why")
	assert.True(t, since.Add(10*time.Minute).Equal(resumeAt), "until when")
}
