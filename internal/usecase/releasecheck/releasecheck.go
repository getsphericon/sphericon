// Package releasecheck is the use case of the Release check (ADR 0032, ADR 0102): once a day it
// asks where the newest published release is, stores a newer one in the Instance state
// (internal/domain/instance/instancestates) and logs it, and it tells a User who holds release:read in a live
// Workspace (an owner, ADR 0061) of the newer release the check found. The rule of which stored
// release still counts is the Instance state's. It takes the Instance state module rather than an
// *ent.Scoped: the Release check belongs to no Workspace.
package releasecheck

import (
	"context"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
)

// releaseLookupTimeout bounds one ask: the source's own client waits much longer.
const releaseLookupTimeout = 30 * time.Second

// ReleaseSource answers "what is the newest stable release", as a bare version: the one
// lookup `sphericon upgrade` builds, so there is no second way to ask (ADR 0032).
type ReleaseSource interface {
	LatestRelease(ctx context.Context) (string, error)
}

// Release is what a Workspace's owner sees of the Instance: the Release check's result.
type Release struct {
	// LatestRelease is the bare version of a newer stable release, nil when there is none.
	LatestRelease *string
}

// UseCase is the Release check over the Instance state.
type UseCase struct {
	states *instancestates.Module
	source ReleaseSource
	log    *slog.Logger
}

// Option configures the use case.
type Option func(*UseCase)

// WithLogger sets the logger of the Release check's lines; the default is slog's.
func WithLogger(l *slog.Logger) Option { return func(u *UseCase) { u.log = l } }

// New builds the Release check over the Instance state, asking source for the newest release.
// Without a source the check does nothing.
func New(states *instancestates.Module, source ReleaseSource, opts ...Option) *UseCase {
	u := &UseCase{states: states, source: source, log: slog.Default()}
	for _, o := range opts {
		o(u)
	}
	return u
}

// ReleaseFor is the banner of a newer release for the User userID: a User who holds the
// Permission release:read (an owner, ADR 0061) in a live Workspace is told, everyone else gets
// that Permission's refusal (403 permission_required). The Instance's settings are not a
// Workspace's business: they are the Staff console's (ADR 0063).
func (u *UseCase) ReleaseFor(ctx context.Context, userID int64) (Release, error) {
	held, err := u.states.HoldsReleaseRead(ctx, userID)
	if err != nil {
		return Release{}, err
	}
	if !held {
		return Release{}, memberships.Refusal(memberships.PermReleaseRead)
	}
	latest, err := u.LatestRelease(ctx)
	if err != nil {
		return Release{}, err
	}
	return Release{LatestRelease: lo.EmptyableToPtr(latest)}, nil
}

// LatestRelease is the cached result of the Release check: a stable release newer than the
// running version, or empty. It asks nobody.
func (u *UseCase) LatestRelease(ctx context.Context) (string, error) {
	return u.states.LatestRelease(ctx)
}

// CheckRelease asks the source for the newest release and stores it with the check time only
// when it is strictly newer than the running version and not a prerelease (the Instance state's
// rule). A source that cannot be reached leaves the previous result and fails nothing. A build
// that is not a release, or an Instance in the saas Mode, asks nobody.
func (u *UseCase) CheckRelease(ctx context.Context) error {
	if u.source == nil || !u.states.ChecksReleases() {
		return nil
	}
	ask, stop := context.WithTimeout(ctx, releaseLookupTimeout)
	defer stop()
	latest, err := u.source.LatestRelease(ask)
	if err != nil {
		u.log.DebugContext(ctx, "release check: source unreachable, keeping the previous result", "err", err)
		return nil
	}
	changed, err := u.states.RecordRelease(ctx, latest)
	if err != nil {
		return err
	}
	if changed {
		u.announce(ctx, latest)
	}
	return nil
}

// AnnounceRelease logs the cached newer release, if any: called at start, so a Self-hoster's
// logs show it before the next daily check.
func (u *UseCase) AnnounceRelease(ctx context.Context) error {
	latest, err := u.LatestRelease(ctx)
	if err != nil || latest == "" {
		return err
	}
	u.announce(ctx, latest)
	return nil
}

func (u *UseCase) announce(ctx context.Context, latest string) {
	u.log.InfoContext(ctx, "a newer release is available; run `sphericon upgrade --check` for details",
		"latest", latest, "running", buildinfo.Bare(u.states.Environment().Version))
}
