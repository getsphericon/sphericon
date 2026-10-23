package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/machine"
)

// privateRelayRefreshInterval is how often the published iCloud Private Relay egress ranges
// are fetched again. Apple changes the list rarely; a day keeps detection current.
const privateRelayRefreshInterval = 24 * time.Hour

// RefreshPrivateRelayArgs is the periodic tick that refreshes the Private Relay ranges that
// machine detection reads (ADR 0053).
type RefreshPrivateRelayArgs struct{}

func (RefreshPrivateRelayArgs) Kind() string { return "private_relay_refresh" }

// RefreshPrivateRelayWorker fetches the published list into the ranges table. A failed fetch
// returns the error, so river retries with its backoff, and the previous ranges stay in place.
type RefreshPrivateRelayWorker struct {
	river.WorkerDefaults[RefreshPrivateRelayArgs]

	ranges *machine.Detector
}

func (w *RefreshPrivateRelayWorker) Work(ctx context.Context, _ *river.Job[RefreshPrivateRelayArgs]) error {
	if err := w.ranges.Refresh(ctx); err != nil {
		slog.WarnContext(ctx, "private relay ranges not refreshed, keeping the previous ones", "err", err)
		return err
	}
	slog.InfoContext(ctx, "private relay ranges refreshed")
	return nil
}
