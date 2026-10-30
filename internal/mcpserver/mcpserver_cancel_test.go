package mcpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/mcpserver"
)

// blockingAPI holds every call until the call's context ends, and reports whether it ended or
// gave up waiting.
type blockingAPI struct {
	entered chan struct{}
	ended   chan bool
}

func newBlockingAPI() *blockingAPI {
	return &blockingAPI{entered: make(chan struct{}), ended: make(chan bool, 1)}
}

func (a *blockingAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	close(a.entered)
	select {
	case <-r.Context().Done():
		a.ended <- true
	case <-time.After(5 * time.Second):
		a.ended <- false
	}
	w.WriteHeader(http.StatusOK)
}

// A tool call ends with the /mcp request that runs it: when a request timeout or a client that
// hangs up ends the request, the /api call it started is cancelled instead of running on.
func TestAToolCallEndsWithItsRequest(t *testing.T) {
	api := newBlockingAPI()
	h, err := mcpserver.New([]byte(specDoc), api, fakeAuth{})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"things_get","arguments":{"id":"1"}}}`
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	go func() {
		<-api.entered
		cancel()
	}()

	h.ServeHTTP(httptest.NewRecorder(), req)

	assert.True(t, <-api.ended, "the /api call is cancelled with the request")
}
