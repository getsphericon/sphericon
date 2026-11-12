package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A token sets, lists, verifies and removes the Workspace's Tracking domain (ADR 0071), with the
// CNAME to publish at the Instance's edge host.
func TestExternalTrackingDomainLifecycle(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeTrackingDomainsRead, apitokens.ScopeTrackingDomainsWrite)
	ctx := context.Background()

	set, err := c.TrackingDomainsSet(ctx, &externalapi.SetTrackingDomainInput{Host: "links.acme.com"})
	require.NoError(t, err)
	assert.Equal(t, externalapi.TrackingDomainRoleCurrent, set.Role)
	assert.Equal(t, externalapi.TrackingStatePending, set.State)
	assert.Equal(t, externalapi.CnameRecordTypeCNAME, set.Record.Type)
	assert.Equal(t, testhelper.TrackingCNAMETarget, set.Record.Target)

	list, err := c.TrackingDomainsList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "links.acme.com", list[0].Host)

	_, err = c.TrackingDomainsVerify(ctx, externalapi.TrackingDomainsVerifyParams{ID: set.ID})
	require.NoError(t, err)

	err = c.TrackingDomainsRemove(ctx, externalapi.TrackingDomainsRemoveParams{ID: set.ID})
	require.NoError(t, err)
}

func TestExternalTrackingDomainsNeedTheirScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	reader := env.ExternalScoped(t, apitokens.ScopeTrackingDomainsRead)
	_, err := reader.TrackingDomainsSet(ctx, &externalapi.SetTrackingDomainInput{Host: "links.acme.com"})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	writer := env.ExternalScoped(t, apitokens.ScopeTrackingDomainsWrite)
	_, err = writer.TrackingDomainsList(ctx)
	testhelper.ExternalRefusal(t, err, http.StatusForbidden, "write does not imply read")
}
