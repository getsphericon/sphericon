package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/broadcast"
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

	c := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID)
	b, err := env.DB.Broadcast.Create().
		SetWorkspaceID(fixtures.AcmeID).SetName("Track").SetSubject("Hi").
		SetStatus(broadcast.StatusSent).Save(ctx)
	require.NoError(t, err)
	rec, err := env.DB.BroadcastRecipient.Create().
		SetBroadcastID(b.ID).SetWorkspaceID(fixtures.AcmeID).SetContactID(c.ID).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Broadcast().Discard(ctx, b.ID))
	token, err := tr.Token(rec.ID)
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/e/o/"+token, nil)
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	opened := env.DB.BroadcastRecipient.GetX(ctx, rec.ID).OpenedAt
	assert.NotNil(t, opened)
	assert.Equal(t, 1, figures(t, env, b.ID).Opened)
}
