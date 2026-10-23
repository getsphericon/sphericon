package sidelistener_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/sidelistener"
)

func TestServerBindsServesAndStops(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /service/ping", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "pong") })
	s := sidelistener.New("internal", "127.0.0.1:0", mux)
	require.NoError(t, s.Listen(t.Context()))
	served := make(chan error, 1)
	go func() { served <- s.Serve() }()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+s.Addr()+"/service/ping", http.NoBody)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "pong", string(body))

	require.NoError(t, s.Shutdown(t.Context()))
	serveErr := testhelper.Receive(t, served, "Serve did not return after Shutdown")
	require.NoError(t, serveErr, "a graceful Shutdown ends Serve without error")
}

func TestListenFailsWhenTheAddressIsTaken(t *testing.T) {
	first := sidelistener.New("internal", "127.0.0.1:0", http.NotFoundHandler())
	require.NoError(t, first.Listen(t.Context()))
	t.Cleanup(func() { cleanup.Report("shut first down", first.Shutdown(t.Context())) })

	second := sidelistener.New("internal", first.Addr(), http.NotFoundHandler())
	require.Error(t, second.Listen(t.Context()))
}
