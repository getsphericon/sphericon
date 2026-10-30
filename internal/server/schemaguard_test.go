package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/server"
)

type pendingFake struct {
	pending int
	err     error
}

func (p pendingFake) Pending(context.Context) (int, error) { return p.pending, p.err }

func serveThroughSchemaGuard(t *testing.T, watch server.PendingMigrations, path string) *httptest.ResponseRecorder {
	t.Helper()
	return sendThroughSchemaGuard(t, watch, http.MethodGet, path)
}

func sendThroughSchemaGuard(t *testing.T, watch server.PendingMigrations, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	w := httptest.NewRecorder()
	server.SchemaGuard(watch)(next).ServeHTTP(w, req)
	return w
}

func TestSchemaGuardAnswersAProblemThatNamesMigrateWhileMigrationsArePending(t *testing.T) {
	w := serveThroughSchemaGuard(t, pendingFake{pending: 3}, "/site/workspaces")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Equal(t, "5", w.Header().Get("Retry-After"))
	assert.Contains(t, w.Body.String(), "3 migration(s) behind")
	assert.Contains(t, w.Body.String(), "mise run db:migrate")
	assert.Contains(t, w.Body.String(), `"code":"migrations_pending"`)
}

// The error page's button is what runs the migrations, so it passes; nothing else on its path does.
func TestSchemaGuardLetsTheButtonThatRunsTheMigrationsThrough(t *testing.T) {
	button := sendThroughSchemaGuard(t, pendingFake{pending: 3}, http.MethodPost, "/site/schema/migrations")
	assert.Equal(t, http.StatusNoContent, button.Code)

	read := sendThroughSchemaGuard(t, pendingFake{pending: 3}, http.MethodGet, "/site/schema/migrations")
	assert.Equal(t, http.StatusServiceUnavailable, read.Code)
}

func TestSchemaGuardPassesWhenTheSchemaIsCurrentOrTheLookupFails(t *testing.T) {
	current := serveThroughSchemaGuard(t, pendingFake{}, "/site/workspaces")
	assert.Equal(t, http.StatusNoContent, current.Code)

	unknown := serveThroughSchemaGuard(t, pendingFake{pending: 3, err: errors.New("db down")}, "/site/workspaces")
	assert.Equal(t, http.StatusNoContent, unknown.Code)
}

func TestSchemaGuardLeavesTheProbesAlone(t *testing.T) {
	for _, path := range []string{"/healthz", "/readyz"} {
		w := serveThroughSchemaGuard(t, pendingFake{pending: 3}, path)
		assert.Equal(t, http.StatusNoContent, w.Code, path)
	}
}
