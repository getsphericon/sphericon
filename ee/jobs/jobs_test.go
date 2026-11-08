package jobs_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ee/jobs"
)

func TestTheRunnerRegistersTheAbuseDetectorNextToTheOtherJobs(t *testing.T) {
	ext := jobs.Extension(jobs.Deps{Clock: time.Now})

	assert.Len(t, ext.PeriodicJobs, 4, "retention, usage, the abuse detector and the Erasures stream's prune")
	assert.NotPanics(t, func() { ext.Workers(river.NewWorkers()) })
}
