package tracking_test

import (
	"context"
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

// Opens and clicks on an archived broadcast keep being recorded (ADR 0049): engagement is
// not lost when the owner tidies up.
func TestTrackingKeepsRecordingForAnArchivedBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	tr := messagetoken.New(cfg.JWTSecret, cfg.AppURL, time.Now)

	b := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastSentUntrackedID)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Broadcast().Archive(ctx, b.ID, time.Now()))
	token, err := tr.Token(fixtures.OutboundMessageTrackingSampleID)
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/e/o/"+token, nil)
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	opened := env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageTrackingSampleID).OpenedAt
	assert.NotNil(t, opened)
	assert.Equal(t, 1, figures(t, env, b.ID).Opened)
}
