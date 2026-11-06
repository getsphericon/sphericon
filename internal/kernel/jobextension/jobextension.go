// Package jobextension is what a package outside the job runner (internal/jobs) hands it to run:
// its workers and its periodic jobs. It is a leaf, so the Enterprise Edition (ADR 0014) and the
// Instance's own jobs declare theirs without importing the runner, which imports the domain
// modules; the composition root passes them to jobs.NewClient.
package jobextension

import "github.com/riverqueue/river"

// Extension plugs extra workers and periodic jobs into the job client.
type Extension struct {
	Workers      func(*river.Workers)
	PeriodicJobs []*river.PeriodicJob
}
