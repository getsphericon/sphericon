package telemetry

import (
	"net/http"

	"github.com/getsphericon/sphericon/internal/sidelistener"
)

// MetricsServer is the opt-in Prometheus listener (ADR 0018): the only place
// /metrics is served. A nil *MetricsServer means the listener is disabled: every
// method is a no-op on it.
type MetricsServer = sidelistener.Server

// NewMetricsServer builds a server for addr (host:port) serving only /metrics.
func NewMetricsServer(addr string) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", MetricsHandler())
	return sidelistener.New("metrics", addr, mux)
}
