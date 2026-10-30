package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// The Instance's settings are the Staff console's (ADR 0063); a Workspace Owner sees only the
// banner of a newer release (ADR 0032).

// latestRelease is the release banner as the caller reads it.
func latestRelease(t *testing.T, c *siteapi.Client) siteapi.OptString {
	t.Helper()
	res, err := c.SiteInstanceGetRelease(t.Context())
	require.NoError(t, err)
	release, ok := res.(*siteapi.SiteInstanceRelease)
	require.True(t, ok, "expected the release, got %T", res)
	return release.LatestRelease
}

// requireReleaseRefused asserts the caller holds release:read in no Workspace.
func requireReleaseRefused(t *testing.T, c *siteapi.Client) {
	t.Helper()
	res, err := c.SiteInstanceGetRelease(t.Context())
	require.NoError(t, err)
	refused, ok := res.(*siteapi.ProblemDetails)
	require.True(t, ok, "expected a refusal, got %T", res)
	assert.Equal(t, siteapi.ProblemCodePermissionRequired, refused.Code.Value)
}

// John owns Acme, the oldest Workspace, and Jane owns Globex: an owner of any Workspace is told.
// Mary is a member of both and Oscar of none: neither is.
func TestTheReleaseBannerReachesAnOwnerOfAnyWorkspaceAndNoOneElse(t *testing.T) {
	_, url := newReleases(t, "v0.2.0")
	env, _ := releaseEnv(t, "0.1.0", url)
	require.NoError(t, env.Instance.CheckRelease(t.Context()))

	john := latestRelease(t, env.SiteActor(t, fixtures.OwnerJohnEmail))
	jane := latestRelease(t, env.SiteActor(t, fixtures.OwnerJaneEmail))

	assert.Equal(t, "0.2.0", john.Value)
	assert.Equal(t, "0.2.0", jane.Value)
	requireReleaseRefused(t, env.SiteActor(t, fixtures.MemberMaryEmail))
	requireReleaseRefused(t, env.SiteActor(t, fixtures.OutsiderOscarEmail))
	_, err := env.SiteAnonymous(t).SiteInstanceGetRelease(t.Context())
	require.Error(t, err)
}
