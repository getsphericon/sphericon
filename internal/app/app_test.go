package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

func TestNewWiresConfigAndServer(t *testing.T) {
	a := newApp(t)

	require.NotNil(t, a.Config)
	assert.True(t, a.Config.IsDev)
	assert.Equal(t, ":"+a.Config.Port, a.Server.Addr)
	require.NotNil(t, a.Server.Handler)

	// The wired handler is the real server: the SPA API refuses an anonymous caller.
	rec := httptest.NewRecorder()
	a.Server.Handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/site/workspaces", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestNewFailsOnABadEncryptionKey(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_ENCRYPTION_KEY", "not-a-keyset")
	_, err := app.New("test")
	require.Error(t, err)
}

func TestShutdownIsIdempotentAndReportsOnce(t *testing.T) {
	a := newApp(t)
	first := a.Shutdown(context.Background())
	require.NotNil(t, first)
	assert.Empty(t, first.Errors)
	assert.Same(t, first, a.Shutdown(context.Background()), "a second shutdown returns the first report")
}

func TestCommandAppHasNothingToServe(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_METRICS_ADDR", freeAddr(t))
	a, err := app.NewCommandApp("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })

	require.NotNil(t, a.Config)
	assert.Nil(t, a.Server)
	assert.Nil(t, a.Metrics, "staff commands expose no metrics listener")
	assert.ErrorIs(t, a.Start(context.Background()), app.ErrNotServable)
}

// Each module and use case a staff member command calls comes from the command app's container.
func TestCommandAppBuildsWhatTheCommandsCall(t *testing.T) {
	baseline(t)
	a, err := app.NewCommandApp("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })

	built := func(v any, err error) any {
		t.Helper()
		require.NoError(t, err)
		return v
	}
	assert.NotNil(t, built(a.Suspender()))
	assert.NotNil(t, built(a.SecondFactors()))
	assert.NotNil(t, built(a.SignUp()))
	assert.NotNil(t, built(a.StaffMembers()))
	assert.NotNil(t, built(a.ServiceCredentials()))
	assert.NotNil(t, built(a.SecretReencryption()))
	assert.NotNil(t, built(a.Workspaces()))
	assert.NotNil(t, built(a.Bus()))
}

func TestE2EDNSOptionIsRefusedOutsideTheE2EProfile(t *testing.T) {
	for _, env := range []string{"production", "development", "test", ""} {
		t.Run(env, func(t *testing.T) {
			a, err := app.New(env, app.WithE2EDNS())
			require.ErrorIs(t, err, app.ErrE2EOnly)
			assert.Nil(t, a)
		})
	}
}
