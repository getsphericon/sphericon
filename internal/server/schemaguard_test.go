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
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
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
