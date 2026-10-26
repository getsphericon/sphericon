package site_test

import (
	"context"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Broadcast's From is a verified Sending domain of the Workspace (ADR 0010): setting another
// one is a 422 on create and on update, empty stays valid, and an update that leaves the From
// alone is not re-validated, so a draft whose domain lost verification stays editable.
func TestSiteBroadcastsFromEmailMustBeAVerifiedSendingDomain(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	createParams := siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID}
	updateParams := siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.BroadcastDraftID}

	create := func(from string) siteapi.SiteBroadcastsCreateRes {
		res, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{
			Name: "from check", FromEmail: siteapi.NewOptNilEmailAddress(siteapi.EmailAddress(from)),
		}, createParams)
		require.NoError(t, err)
		return res
	}
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, create("news@"+fixtures.SendingDomainVerifiedDomain))
	for _, from := range []string{
		"news@" + fixtures.SendingDomainUnverifiedDomain,
		"news@" + fixtures.SendingDomainGlobexDomain,
		"news@" + fixtures.SendingDomainArchivedDomain,
		"news@nowhere.example",
	} {
		assert.IsType(t, &siteapi.SiteBroadcastsCreateUnprocessableEntity{}, create(from), from)
	}
	refused := create("news@" + fixtures.SendingDomainUnverifiedDomain)
	problem, ok := refused.(*siteapi.SiteBroadcastsCreateUnprocessableEntity)
	require.True(t, ok)
	assert.Contains(t, problem.Detail.Or(""), fixtures.SendingDomainUnverifiedDomain, "the refusal names the domain")

	res, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{Name: "no from"}, createParams)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, res)

	badUpd, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{
		FromEmail: siteapi.NewOptNilEmailAddress("news@" + fixtures.SendingDomainUnverifiedDomain),
	}, updateParams)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, badUpd)
	okUpd, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{
		FromEmail: siteapi.NewOptNilEmailAddress("news@" + fixtures.SendingDomainVerifiedDomain),
	}, updateParams)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, okUpd)

	// The draft's From lost its verification: the subject is still editable, with or without
	// the unchanged From in the request.
	lost := siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.BroadcastDraftFromLostDomainID}
	for _, in := range []*siteapi.SiteUpdateBroadcastInput{
		{Subject: siteapi.NewOptString("New subject")},
		{
			Subject:   siteapi.NewOptString("Newer subject"),
			FromEmail: siteapi.NewOptNilEmailAddress("hello@" + fixtures.SendingDomainLostDomain),
		},
	} {
		got, err := c.SiteBroadcastsUpdate(ctx, in, lost)
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastResource{}, got)
	}
}
