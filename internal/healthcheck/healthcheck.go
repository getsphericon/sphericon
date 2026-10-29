// Package healthcheck is `sphericon healthcheck`, the image's HEALTHCHECK. The runtime image is
// scratch, with no shell and no wget, so the binary probes the /healthz of the server on this
// host itself.
package healthcheck

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// timeout bounds a probe: a server that does not answer within it is not healthy.
const timeout = 2 * time.Second

// Probe asks the server on port of this host for /healthz through client and returns nil only
// when it answers 200.
func Probe(ctx context.Context, client *http.Client, port string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := "http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if _, err := io.Copy(io.Discard, res.Body); err != nil {
		return fmt.Errorf("healthcheck: read the answer: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: GET %s answered %s", url, res.Status)
	}
	return nil
}
