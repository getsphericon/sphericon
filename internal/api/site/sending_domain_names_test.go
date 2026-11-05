package site_test

import (
	"context"
	"net/http"
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
	create := func(domain string) (*siteapi.SiteSendingDomainResource, error) {
		return c.SiteSendingDomainsCreate(ctx, &siteapi.CreateSendingDomainInput{Domain: domain},
			siteapi.SiteSendingDomainsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	}

	res, err := create("Почта.Пример.рф")
	require.NoError(t, err, "an internationalized domain is accepted")
	assert.Equal(t, "xn--80a1acny.xn--e1afmkfd.xn--p1ai", res.Domain, "stored as punycode")

	for _, bad := range []string{"192.0.2.1", "-bad.example.com", "exa mple.com", "https://example.com", "under_score.example.com"} {
		_, err := create(bad)
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity, bad)
	}
}
