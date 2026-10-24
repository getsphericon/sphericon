package site_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAContactCreatedByHandInTheSiteCarriesTheManualSource(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := c.SiteContactsCreate(ctx, &siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress("source-hand@example.com")}, siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	created, err := env.DB.Contact.Query().Where(contact.Email("source-hand@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, contact.SourceManual, lo.FromPtr(created.Source))
}
