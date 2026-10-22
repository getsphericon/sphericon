package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// Opens and clicks on an archived broadcast keep being recorded (ADR 0049): engagement is
// not lost when the owner tidies up.
func TestTrackingKeepsRecordingForAnArchivedBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	tr := tracking.New(cfg.JWTSecret, cfg.AppURL)

	b := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastSentUntrackedID)
	rec := env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientUntrackedID)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Broadcast().Discard(ctx, b.ID))
	token, err := tr.Token(fixtures.OutboundMessageTrackingSampleID)
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/e/o/"+token, nil)
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	opened := env.DB.BroadcastRecipient.GetX(ctx, rec.ID).OpenedAt
	assert.NotNil(t, opened)
	assert.Equal(t, 1, figures(t, env, b.ID).Opened)
}
