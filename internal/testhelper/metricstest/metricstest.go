package metricstest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/telemetry"
)

// Scrape is one Prometheus exposition body taken from the real /metrics handler.
type Scrape string

// StartMetrics installs the real telemetry pipeline (global OTel providers plus
// the Prometheus handler) for this test, so instruments registered afterwards
// show up in Scrape. Shutdown is registered as cleanup.
func StartMetrics(t *testing.T) {
	t.Helper()
	stop, err := telemetry.Setup(context.Background(), &config.Config{OtelServiceName: "sphericon-test"}, "test", telemetry.BuildInfo{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = stop(context.Background()) })
}

// ScrapeMetrics reads /metrics through the same handler the metrics listener serves.
func ScrapeMetrics(t *testing.T) Scrape {
	t.Helper()
	rec := httptest.NewRecorder()
	telemetry.MetricsHandler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(rec.Result().Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, string(body))
	return Scrape(body)
}

// find returns the sample line of the metric whose labels include every
// key/value of labels (other labels, such as the OTel scope ones, are ignored).
func (s Scrape) find(name string, labels map[string]string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(string(s)))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, name+"{") && !strings.HasPrefix(line, name+" ") {
			continue
		}
		match := true
		for k, v := range labels {
			if !strings.Contains(line, fmt.Sprintf(`%s=%q`, k, v)) {
				match = false
				break
			}
		}
		if match {
			return line, true
		}
	}
	return "", false
}

// Has reports whether the scrape carries a sample of name matching labels.
func (s Scrape) Has(name string, labels map[string]string) bool {
	_, ok := s.find(name, labels)
	return ok
}

// Value returns the sample value of name matching labels; the test fails when
// there is none.
func (s Scrape) Value(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	line, ok := s.find(name, labels)
	if !ok {
		require.Failf(t, "metric not found", "%s%v in scrape:\n%s", name, labels, string(s))
	}
	fields := strings.Fields(line)
	val, err := strconv.ParseFloat(fields[len(fields)-1], 64)
	require.NoError(t, err, line)
	return val
}
