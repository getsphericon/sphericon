package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A token sets, lists, verifies and removes the Workspace's Tracking domain (ADR 0071), with the
// CNAME to publish at the Instance's edge host.
func TestExternalTrackingDomainLifecycle(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "tracking_domains:read", "tracking_domains:write")
	ctx := context.Background()

	res, err := c.TrackingDomainsSet(ctx, &externalapi.SetTrackingDomainInput{Host: "links.acme.com"})
	require.NoError(t, err)
	set, ok := res.(*externalapi.TrackingDomainResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, externalapi.TrackingDomainRoleCurrent, set.Role)
	assert.Equal(t, externalapi.TrackingStatePending, set.State)
	assert.Equal(t, externalapi.CnameRecordTypeCNAME, set.Record.Type)
	assert.Equal(t, testhelper.TrackingCNAMETarget, set.Record.Target)

	listed, err := c.TrackingDomainsList(ctx)
	require.NoError(t, err)
	list, ok := listed.(*externalapi.TrackingDomainsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", listed)
	require.Len(t, *list, 1)
	assert.Equal(t, "links.acme.com", (*list)[0].Host)

	verified, err := c.TrackingDomainsVerify(ctx, externalapi.TrackingDomainsVerifyParams{ID: set.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TrackingDomainResource{}, verified)

	removed, err := c.TrackingDomainsRemove(ctx, externalapi.TrackingDomainsRemoveParams{ID: set.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TrackingDomainsRemoveNoContent{}, removed)
}

func TestExternalTrackingDomainsNeedTheirScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	reader := env.ExternalScoped(t, "tracking_domains:read")
	set, err := reader.TrackingDomainsSet(ctx, &externalapi.SetTrackingDomainInput{Host: "links.acme.com"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TrackingDomainsSetForbidden{}, set)

	writer := env.ExternalScoped(t, "tracking_domains:write")
	list, err := writer.TrackingDomainsList(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TrackingDomainsListForbidden{}, list, "write does not imply read")
}
