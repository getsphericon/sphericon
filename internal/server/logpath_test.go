package server_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// credentialRoutes are the routes that carry a credential in the path: the signed tracking,
// unsubscribe and confirm tokens, the invitation token and the ingest hook key.
var credentialRoutes = []string{
	"/e/o/", "/e/c/", "/e/u/", "/e/confirm/", "/hooks/",
	"/site/confirmations/", "/site/unsubscribes/", "/site/invitations/",
}

// loggedPathOf serves one GET of path through h and returns the path its access log line names,
// whatever its level.
func loggedPathOf(ctx context.Context, t require.TestingT, h http.Handler, path string) any {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil))

	return onlyLine(t, &buf, "request")["path"]
}

func TestTheLoggedPathNeverKeepsACredential(t *testing.T) {
	h := internalServer(t, statusHandler(http.StatusOK))

	rapid.Check(t, func(rt *rapid.T) {
		route := rapid.SampledFrom(credentialRoutes).Draw(rt, "route")
		credential := rapid.StringMatching(`[A-Za-z0-9._-]{8,64}(/accept)?`).Draw(rt, "credential")

		logged := loggedPathOf(t.Context(), rt, h, route+credential)

		assert.Equal(rt, route+"{redacted}", logged)
	})
}

func TestTheLoggedPathKeepsEveryOtherPath(t *testing.T) {
	h := internalServer(t, statusHandler(http.StatusOK))

	rapid.Check(t, func(rt *rapid.T) {
		root := rapid.SampledFrom([]string{"/site/workspaces/", "/api/", "/collect/", "/staff/", "/service/", "/oauth/"}).Draw(rt, "root")
		path := root + rapid.StringMatching(`[a-z0-9/_-]{0,40}`).Draw(rt, "rest")

		logged := loggedPathOf(t.Context(), rt, h, path)

		assert.Equal(rt, path, logged)
	})
}

// A Workspace's invitations are not the invitation token's route.
func TestTheLoggedPathLeavesWorkspaceInvitationsAlone(t *testing.T) {
	h := internalServer(t, statusHandler(http.StatusOK))

	logged := loggedPathOf(t.Context(), t, h, "/site/workspaces/AbCd1234/invitations/42")

	assert.Equal(t, "/site/workspaces/AbCd1234/invitations/42", logged)
}
