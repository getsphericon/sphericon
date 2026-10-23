package testhelper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/cleanup"
)

// MCPEndpoint is the in-memory URL of the MCP surface (the host is a placeholder;
// requests never leave the process).
const MCPEndpoint = "http://local/mcp"

// MCPClient connects the official go-sdk MCP client to /mcp over the in-memory
// app (no socket) and returns the live session. An empty token connects without
// an Authorization header. The session closes with the test.
func (env *TestEnv) MCPClient(t *testing.T, token string) *mcp.ClientSession {
	t.Helper()
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             MCPEndpoint,
		HTTPClient:           &http.Client{Transport: handlerTripper{handler: env.Server, headers: headers}},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "sphericon-test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), transport, nil)
	require.NoError(t, err, "connect MCP client")
	t.Cleanup(func() { cleanup.Report("close the MCP session", session.Close()) })
	return session
}

// handlerTripper is an http.RoundTripper that serves requests straight from an
// http.Handler.
type handlerTripper struct {
	handler http.Handler
	headers map[string]string
}

func (h handlerTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h.headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, r)
	return rec.Result(), nil
}
