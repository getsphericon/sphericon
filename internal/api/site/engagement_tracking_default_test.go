package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func setEngagementTrackingDefault(t *testing.T, env *testhelper.TestEnv, c *siteapi.Client, d siteapi.EngagementTrackingDecision) (*siteapi.SiteWorkspaceResource, error) {
	t.Helper()
	return c.SiteWorkspacesSetEngagementTrackingDefault(t.Context(),
		&siteapi.SiteSetEngagementTrackingDefaultInput{Revision: acmeRevision(t, env), EngagementTrackingDefault: d},
		siteapi.SiteWorkspacesSetEngagementTrackingDefaultParams{WorkspaceId: fixtures.AcmePublicID})
}

// A Workspace tracks by default until it changes the default (ADR 0120).
func TestAWorkspaceTracksByDefault(t *testing.T) {
	env := testhelper.Setup(t)

	got, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteWorkspacesGet(t.Context(), siteapi.SiteWorkspacesGetParams{WorkspaceId: fixtures.AcmePublicID})

	require.NoError(t, err)
	assert.Equal(t, siteapi.EngagementTrackingDecisionTrack, got.EngagementTrackingDefault)
}

// An Owner changes the Engagement tracking default, and the Workspace reads it on /site and /api.
func TestAnOwnerSetsTheEngagementTrackingDefault(t *testing.T) {
	env := testhelper.Setup(t)

	set, err := setEngagementTrackingDefault(t, env, env.SiteActor(t, fixtures.OwnerJohnEmail), siteapi.EngagementTrackingDecisionDontTrack)

	require.NoError(t, err)
	assert.Equal(t, siteapi.EngagementTrackingDecisionDontTrack, set.EngagementTrackingDefault)
	read, err := env.ExternalScoped(t, apitokens.ScopeContactsRead).WorkspaceGet(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, "dont_track", read.EngagementTrackingDefault)
}

// A Member may not change the default.
func TestAMemberCannotSetTheEngagementTrackingDefault(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := setEngagementTrackingDefault(t, env, env.SiteActor(t, fixtures.MemberMaryEmail), siteapi.EngagementTrackingDecisionDontTrack)

	testhelper.SiteRefusal(t, err, http.StatusForbidden)
}
