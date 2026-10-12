package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A sending domain is stored in its ASCII (punycode) form, the one DNS and DKIM use;
// names that are not valid registrable domains are rejected.
func TestSiteSendingDomainsCreateNormalizesAndValidatesNames(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	create := func(domain string) siteapi.SiteSendingDomainsCreateRes {
		res, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: domain},
			siteapi.SiteSendingDomainsCreateParams{Slug: fixtures.AcmeSlug})
		require.NoError(t, err)
		return res
	}

	res, ok := create("Почта.Пример.рф").(*siteapi.SiteSendingDomainResource)
	require.True(t, ok, "an internationalized domain is accepted")
	assert.Equal(t, "xn--80a1acny.xn--e1afmkfd.xn--p1ai", res.Domain, "stored as punycode")

	for _, bad := range []string{"192.0.2.1", "-bad.example.com", "exa mple.com", "https://example.com", "under_score.example.com"} {
		assert.IsType(t, &siteapi.SiteSendingDomainsCreateUnprocessableEntity{}, create(bad), bad)
	}
}
