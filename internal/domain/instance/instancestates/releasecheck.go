package instancestates

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
)

// releaseLookupTimeout bounds one ask: the source's own client waits much longer.
const releaseLookupTimeout = 30 * time.Second

// ReleaseSource answers "what is the newest stable release", as a bare version: the one
// lookup `sphericon upgrade` builds, so there is no second way to ask (ADR 0032). It is the
// Release check's outside party: production asks the release host, a test a source of its own.
type ReleaseSource interface {
	LatestRelease(ctx context.Context) (string, error)
}

// Release is what a Workspace's owner sees of the Instance: the Release check's result.
type Release struct {
	// LatestRelease is the bare version of a newer stable release, nil when there is none.
	LatestRelease *string
}

// ReleaseFor is the banner of a newer release for the User userID: a User who holds the
// Permission release:read (an owner, ADR 0061) in a live Workspace is told, everyone else gets
// that Permission's refusal (403 permission_required). The Instance's settings are not a
// Workspace's business: they are the Staff console's (ADR 0063).
func (m *Module) ReleaseFor(ctx context.Context, userID int64) (Release, error) {
	held, err := m.holdsReleaseRead(ctx, userID)
	if err != nil {
		return Release{}, err
	}
	if !held {
		return Release{}, memberships.Refusal(memberships.PermReleaseRead)
	}
	latest, err := m.LatestRelease(ctx)
	if err != nil {
		return Release{}, err
	}
	return Release{LatestRelease: lo.EmptyableToPtr(latest)}, nil
}

// CheckRelease is the Release check (GLOSSARY): it asks the source for the newest release and
// stores it with the check time only when it is strictly newer than the running version and not
// a prerelease, and logs a newer one it had not stored. A source that cannot be reached leaves the
// previous result and fails nothing. A build that is not a release, an Instance in the saas Mode,
// or a module built without a source asks nobody.
func (m *Module) CheckRelease(ctx context.Context) error {
	if m.releases == nil || !m.ChecksReleases() {
		return nil
	}
	ask, stop := context.WithTimeout(ctx, releaseLookupTimeout)
	defer stop()
	latest, err := m.releases.LatestRelease(ask)
	if err != nil {
		m.log.DebugContext(ctx, "release check: source unreachable, keeping the previous result", "err", err)
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
		"latest", latest, "running", buildinfo.Bare(m.env.Version))
}
