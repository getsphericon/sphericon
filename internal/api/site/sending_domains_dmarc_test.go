package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The "re-check now" of a Sending domain answers with its DMARC lookup: when it looked and when it
// found the record (ADR 0012). A missing record never gates sending, and a read of the domain shows
// the same moments. What the Needs attention report makes of them is the use case's.
func TestARecheckAnswersWithTheDMARCLookupAndAReadShowsTheSame(t *testing.T) {
	zone := dnstest.NewZone(t)
	env := testhelper.Setup(t, testhelper.WithTXTLookup(zone.LookupTXT))
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.SendingDomainVerifiedID)}
	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	zone.PublishTXT(t, "sphericon._domainkey.mail.acme.com.", dom.DkimPublicKey)

	missing, err := c.SiteSendingDomainsVerify(ctx, params)
	require.NoError(t, err)
	zone.PublishTXT(t, "_dmarc.acme.com.", "v=DMARC1; p=none;")
	published, err := c.SiteSendingDomainsVerify(ctx, params)
	require.NoError(t, err)
	read, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams(params))
	require.NoError(t, err)

	assert.True(t, missing.Verified, "DMARC never gates sending")
	assert.True(t, missing.DmarcCheckedAt.Set, "the DMARC lookup answered")
	assert.False(t, missing.DmarcFoundAt.Set, "the check found no DMARC record")
	assert.True(t, published.DmarcFoundAt.Set, "the organizational domain's record covers mail.acme.com")
	assert.Equal(t, published.DmarcFoundAt, read.DmarcFoundAt, "a read of the domain shows the same moment")
}
