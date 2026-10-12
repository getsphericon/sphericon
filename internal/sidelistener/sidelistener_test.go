package sidelistener_test

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/sidelistener"
)

func TestServerBindsServesAndStops(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /service/ping", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "pong") })
	s := sidelistener.New("internal", "127.0.0.1:0", mux)
	require.NoError(t, s.Listen(t.Context()))
	served := make(chan error, 1)
	go func() { served <- s.Serve() }()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+s.Addr()+"/service/ping", nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "pong", string(body))

	require.NoError(t, s.Shutdown(t.Context()))
	select {
	case err := <-served:
		require.NoError(t, err, "a graceful Shutdown ends Serve without error")
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Shutdown")
	}
}

func TestListenFailsWhenTheAddressIsTaken(t *testing.T) {
	first := sidelistener.New("internal", "127.0.0.1:0", http.NotFoundHandler())
	require.NoError(t, first.Listen(t.Context()))
	t.Cleanup(func() { _ = first.Shutdown(t.Context()) })

	second := sidelistener.New("internal", first.Addr(), http.NotFoundHandler())
	require.Error(t, second.Listen(t.Context()))
}

func TestWithTimeoutsClosesAConnectionThatStallsItsBody(t *testing.T) {
	s := sidelistener.New("internal", "127.0.0.1:0", http.NotFoundHandler(), sidelistener.WithTimeouts(200*time.Millisecond, time.Minute))
	require.NoError(t, s.Listen(t.Context()))
	go func() { _ = s.Serve() }()
	t.Cleanup(func() { _ = s.Shutdown(t.Context()) })

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", s.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	// Complete headers promise a body that never arrives.
	_, err = io.WriteString(conn, "POST /service/x HTTP/1.1\r\nHost: x\r\nContent-Length: 10\r\n\r\n")
	require.NoError(t, err)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	// Whatever the server answers, it then closes: reading ends with EOF, not our deadline.
	_, err = io.ReadAll(bufio.NewReader(conn))
	require.NoError(t, err, "the server closed the stalled connection before the client gave up")
}
