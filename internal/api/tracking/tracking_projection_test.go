package tracking_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestTrackingRecordsItsFactBeforeTheReportProjection(t *testing.T) {
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	tr := messagetoken.New(cfg.JWTSecret, cfg.AppURL, func() time.Time { return at })
	token, err := tr.Token(fixtures.OutboundMessageTrackingSampleID)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/e/o/"+token, nil)
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	before := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundMessageTrackingSampleID)
	assert.Nil(t, before.OpenedAt)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	after := env.DB.OutboundMessage.GetX(t.Context(), fixtures.OutboundMessageTrackingSampleID)
	require.NotNil(t, after.OpenedAt)
	assert.True(t, at.Equal(*after.OpenedAt))
}
