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

// An update made on a revision the Template has moved past is a 409 revision_changed, and the row
// stays as the write before it left it (ADR 0119).
func TestSiteTemplateUpdateOnAStaleRevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteTemplatesUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.TemplateWelcomeID)}
	read, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: fixtures.AcmePublicID, ID: params.ID})
	require.NoError(t, err)

	first, err := c.SiteTemplatesUpdate(ctx, &siteapi.UpdateTemplateInput{Revision: read.Revision, Subject: siteapi.NewOptString("First")}, params)
	require.NoError(t, err)
	_, err = c.SiteTemplatesUpdate(ctx, &siteapi.UpdateTemplateInput{Revision: read.Revision, Subject: siteapi.NewOptString("Second")}, params)

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
	assert.Equal(t, read.Revision+1, first.Revision)
	stored := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)
	assert.Equal(t, "First", stored.Subject)
	assert.EqualValues(t, first.Revision, stored.Revision)
}

// An update that names no revision is refused by the contract and writes nothing.
func TestSiteTemplateUpdateWithoutARevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.DropField("revision"))
	ctx := context.Background()
	before := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)

	_, err := c.SiteTemplatesUpdate(ctx, &siteapi.UpdateTemplateInput{Revision: siteapi.Revision(before.Revision), Subject: siteapi.NewOptString("Blind")},
		siteapi.SiteTemplatesUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.TemplateWelcomeID)})

	testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	after := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)
	assert.Equal(t, before.Subject, after.Subject)
	assert.Equal(t, before.Revision, after.Revision)
}
