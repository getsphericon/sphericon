package instance

import (
	"context"
	"time"

	"github.com/riverqueue/river"
	"golang.org/x/mod/semver"

	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/jobs"
)

// releaseLookupTimeout bounds one ask: the source's own client waits much longer.
const releaseLookupTimeout = 30 * time.Second

// ReleaseSource answers "what is the newest stable release", as a bare version: the one
// lookup `sphericon upgrade` builds, so there is no second way to ask (ADR 0032).
type ReleaseSource interface {
	LatestRelease(ctx context.Context) (string, error)
}

// checking is whether the Release check runs at all: it is switched on, this is not a
// development or test environment and the running build is a release.
func (m *Module) checking() bool {
	return m.env.ReleaseCheck && !m.env.Dev && buildinfo.IsRelease(m.host.AppVersion)
}

// isNewer is whether latest is a stable release strictly newer than the running version.
func (m *Module) isNewer(latest string) bool {
	return buildinfo.IsRelease(latest) && semver.Compare(buildinfo.Semver(latest), buildinfo.Semver(m.host.AppVersion)) > 0
}

// newer is the stored release when it is still newer than the running version (an Instance
// upgraded since the check stops announcing it) and the check is on; empty otherwise.
func (m *Module) newer(stored *string) string {
	if stored == nil || !m.checking() || !m.isNewer(*stored) {
		return ""
	}
	return *stored
}

// LatestRelease is the cached result of the Release check: a stable release newer than the
// running version, or empty. It asks nobody.
func (m *Module) LatestRelease(ctx context.Context) (string, error) {
	row, err := m.Ensure(ctx)
	if err != nil {
		return "", err
	}
	return m.newer(row.LatestRelease), nil
}

// CheckRelease asks the source for the newest release and stores it with the check time only
// when it is strictly newer than the running version and not a prerelease. A source that
// cannot be reached leaves the previous result and fails nothing. A build that is not a
// release, or SPHERICON_RELEASE_CHECK=false, asks nobody.
func (m *Module) CheckRelease(ctx context.Context) error {
	if m.source == nil || !m.checking() {
		return nil
	}
	ask, stop := context.WithTimeout(ctx, releaseLookupTimeout)
	defer stop()
	latest, err := m.source.LatestRelease(ask)
	if err != nil {
		m.log.DebugContext(ctx, "release check: source unreachable, keeping the previous result", "err", err)
		return nil
	}
	if !m.isNewer(latest) {
		return nil
	}
	changed, err := m.recordRelease(ctx, latest)
	if err != nil {
		return err
	}
	if changed {
		m.announce(ctx, latest)
	}
	return nil
}

// AnnounceRelease logs the cached newer release, if any: called at start, so a Self-hoster's
// logs show it before the next daily check.
func (m *Module) AnnounceRelease(ctx context.Context) error {
	latest, err := m.LatestRelease(ctx)
	if err != nil || latest == "" {
		return err
	}
	m.announce(ctx, latest)
	return nil
}

func (m *Module) announce(ctx context.Context, latest string) {
	m.log.InfoContext(ctx, "a newer release is available; run `sphericon upgrade --check` for details",
		"latest", latest, "running", buildinfo.Bare(m.host.AppVersion))
}

// ReleaseCheckArgs is the daily tick of the Release check.
type ReleaseCheckArgs struct{}

func (ReleaseCheckArgs) Kind() string { return "release_check" }

// releaseCheckWorker runs the Release check.
type releaseCheckWorker struct {
	river.WorkerDefaults[ReleaseCheckArgs]
	m *Module
}

func (w *releaseCheckWorker) Work(ctx context.Context, _ *river.Job[ReleaseCheckArgs]) error {
	return w.m.CheckRelease(ctx)
}

// Jobs is the daily Release check, handed to the job client as an extension. A restart
// checks again at once, which is harmless: the check sends nothing.
func (m *Module) Jobs() jobs.Extension {
	return jobs.Extension{
		Workers: func(w *river.Workers) { river.AddWorker(w, &releaseCheckWorker{m: m}) },
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return ReleaseCheckArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
		},
	}
}
