// Package abusewatch is the read seam between the /site API and the Enterprise abuse
// detector (ADR 0026): core knows only this interface and ee implements it, so neither
// imports the other. A nil Watch, or an unlicensed instance, shows no warning.
package abusewatch

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

// Metric is the rate that breached.
type Metric string

const (
	MetricComplaint Metric = "complaint"
	MetricBounce    Metric = "bounce"
)

// Warning is a Sending domain in the detector's warning tier.
type Warning struct {
	Domain string
	Metric Metric
	// Rate is 0–1.
	Rate float64
}

// Watch reads a Workspace's standing with the detector.
type Watch interface {
	// Warning is the Workspace's warning at now, or nil when it is in no warning tier
	// (exempt and suspended Workspaces are never in one).
	Warning(ctx context.Context, s *ent.Scoped, now time.Time) (*Warning, error)
}
