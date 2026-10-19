//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HookTimeout bounds every wait for a call to a Hook; asynchronous steps are polled, never slept.
const HookTimeout = 60 * time.Second

// hookSecret is the signing secret of a signed Hook (a Standard Webhooks whsec_ value).
const hookSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

// HookCall is one request a third-party receiver got.
type HookCall struct {
	Header http.Header
	Body   string
}

// Hooks is the suite's one third-party receiver, the way Mailpit is its one mail catcher.
// It listens on a loopback port that the application's outbound allowlist names (set
// before Boot: the hardened client refuses loopback addresses otherwise), and keeps what
// it got per path, so every test owns a path and never sees another's calls.
type Hooks struct {
	ln  net.Listener
	srv *http.Server

	mu    sync.Mutex
	calls map[string][]HookCall
}

// StartHooks opens the receiver and returns it with its stop function.
func StartHooks(ctx context.Context) (*Hooks, func(), error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("hooks: listen: %w", err)
	}
	h := &Hooks{ln: ln, calls: map[string][]HookCall{}}
	h.srv = &http.Server{Handler: http.HandlerFunc(h.record), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = h.srv.Serve(ln) }()
	return h, func() { _ = h.srv.Close() }, nil
}

// Addr is the receiver's host:port, the entry for SPHERICON_OUTBOUND_ALLOW.
func (h *Hooks) Addr() string { return h.ln.Addr().String() }

func (h *Hooks) record(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	h.mu.Lock()
	h.calls[r.URL.Path] = append(h.calls[r.URL.Path], HookCall{Header: r.Header.Clone(), Body: string(body)})
	h.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (h *Hooks) callsAt(path string) []HookCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]HookCall(nil), h.calls[path]...)
}

// Hook is one test's own endpoint on the receiver. It is the only way a scenario observes
// an HTTP step: one wait, one absence check, like the Inbox.
type Hook struct {
	t      testing.TB
	hooks  *Hooks
	path   string
	URL    string
	Secret string
}

// NewHook gives the test an endpoint of its own. A signed one verifies every call's
// Standard Webhooks signature against Secret; an unsigned one expects none.
func (w *Workspace) NewHook(signed bool) *Hook {
	h := w.env.hooks
	path := "/hook/" + uniq()
	hook := &Hook{t: w.t, hooks: h, path: path, URL: "http://" + h.Addr() + path}
	if signed {
		hook.Secret = hookSecret
	}
	return hook
}

// Wait polls (bounded by HookTimeout) until the endpoint got a call and returns it,
// having checked its signature. On a miss it fails the test.
func (hk *Hook) Wait() HookCall {
	hk.t.Helper()
	var calls []HookCall
	require.Eventually(hk.t, func() bool {
		calls = hk.hooks.callsAt(hk.path)
		return len(calls) > 0
	}, HookTimeout, 100*time.Millisecond, "no call to %s", hk.path)
	call := calls[0]
	signature := call.Header.Get(standardwebhooks.HeaderWebhookSignature)
	if hk.Secret == "" {
		assert.Empty(hk.t, signature, "an HTTP step without a secret sends no signature")
		return call
	}
	wh, err := standardwebhooks.NewWebhook(hk.Secret)
	require.NoError(hk.t, err)
	assert.NoError(hk.t, wh.Verify([]byte(call.Body), call.Header), "the call is signed with the step's secret")
	return call
}

// RequireNone asserts that the endpoint gets no call within the window (default 3 s).
func (hk *Hook) RequireNone(window ...time.Duration) {
	hk.t.Helper()
	w := absenceWindow
	if len(window) > 0 {
		w = window[0]
	}
	assert.Never(hk.t, func() bool { return len(hk.hooks.callsAt(hk.path)) > 0 }, w, 100*time.Millisecond,
		"unexpected call to %s", strings.TrimPrefix(hk.path, "/hook/"))
}
