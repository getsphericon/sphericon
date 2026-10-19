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

// lintBody breaks every warning rule against Acme's saved Brand kit (accent #1a73e8, text
// #202124, Arial) and is valid HTML, so none of it blocks a save.
const lintBody = `<table><tbody><tr><td>` +
	`<img src="https://cdn.example.com/hero.png">` +
	`<p style="color:#cc0000;font-family:Georgia, serif">Sale</p>` +
	`<p style="color:#eeeeee;background-color:#ffffff">Faint print</p>` +
	`<p style="color:#202124">Brand text</p>` +
	`<marquee>Scroll</marquee>` +
	`</td></tr></tbody></table>`

func findings(list []siteapi.LintFinding) map[siteapi.LintCode][]string {
	got := map[siteapi.LintCode][]string{}
	for _, f := range list {
		got[f.Code] = append(got[f.Code], f.Element.Value+" "+f.Detail.Value)
	}
	return got
}

func TestSiteTemplateSaveReturnsWarningsAndStoresTheContent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	created, err := c.SiteTemplatesCreate(ctx, &siteapi.SiteCreateEmailTemplateInput{
		Name: "Warned", Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString(lintBody),
	}, siteapi.SiteTemplatesCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", created)

	got := findings(res.Warnings)
	require.Len(t, got[siteapi.LintCodeMissingAlt], 1)
	assert.Contains(t, got[siteapi.LintCodeMissingAlt][0], "https://cdn.example.com/hero.png")
	require.Len(t, got[siteapi.LintCodeLowContrast], 1)
	assert.Contains(t, got[siteapi.LintCodeLowContrast][0], "Faint print")
	assert.Contains(t, got[siteapi.LintCodeLowContrast][0], "needs 4.5:1")
	require.Len(t, got[siteapi.LintCodeOffBrandColor], 2, "red and light gray; white and #202124 are the kit's")
	assert.Contains(t, got[siteapi.LintCodeOffBrandColor][0], "Sale")
	assert.Contains(t, got[siteapi.LintCodeOffBrandColor][0], "#cc0000")
	assert.Contains(t, got[siteapi.LintCodeOffBrandColor][1], "#eeeeee")
	require.Len(t, got[siteapi.LintCodeOffBrandFont], 1)
	assert.Contains(t, got[siteapi.LintCodeOffBrandFont][0], "Sale")
	require.Len(t, got[siteapi.LintCodeUnknownConstruct], 1)
	assert.Contains(t, got[siteapi.LintCodeUnknownConstruct][0], "marquee")

	// Warnings never block: the template is stored as written (sanitized only).
	row := env.DB.EmailTemplate.GetX(ctx, int64(res.ID))
	assert.Contains(t, row.Body, "Faint print")
	assert.Contains(t, row.Body, "hero.png")
}

func TestSiteTemplateSaveWithoutABrandKitSkipsBrandWarnings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)

	created, err := c.SiteTemplatesCreate(context.Background(), &siteapi.SiteCreateEmailTemplateInput{
		Name: "Warned", Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString(lintBody),
	}, siteapi.SiteTemplatesCreateParams{Slug: fixtures.VandelaySlug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", created)

	got := findings(res.Warnings)
	assert.Len(t, got[siteapi.LintCodeMissingAlt], 1)
	assert.Len(t, got[siteapi.LintCodeLowContrast], 1)
	assert.Len(t, got[siteapi.LintCodeUnknownConstruct], 1)
	assert.Empty(t, got[siteapi.LintCodeOffBrandColor], "no Brand kit saved: nothing to compare to")
	assert.Empty(t, got[siteapi.LintCodeOffBrandFont])
}

func TestSiteTemplateSaveRefusesOnlyWhatCannotRender(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	before := env.DB.EmailTemplate.Query().CountX(ctx)

	res, err := c.SiteTemplatesCreate(ctx, &siteapi.SiteCreateEmailTemplateInput{
		Name: "Broken", Body: siteapi.NewOptString("<div><span>unclosed</div>"),
	}, siteapi.SiteTemplatesCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesCreateUnprocessableEntity{}, res)
	assert.Equal(t, before, env.DB.EmailTemplate.Query().CountX(ctx), "nothing stored")
}

func TestSiteBroadcastSaveReturnsWarnings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{
		Name: "Warned", Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString(lintBody),
	}, siteapi.SiteBroadcastsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", created)
	got := findings(res.Warnings)
	assert.Len(t, got[siteapi.LintCodeMissingAlt], 1)
	assert.Len(t, got[siteapi.LintCodeOffBrandFont], 1)

	// A write of the subject alone lints what is stored.
	updated, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{Subject: siteapi.NewOptString("Hello")},
		siteapi.SiteBroadcastsUpdateParams{Slug: fixtures.AcmeSlug, ID: res.ID})
	require.NoError(t, err)
	upd, ok := updated.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", updated)
	assert.Len(t, findings(upd.Warnings)[siteapi.LintCodeMissingAlt], 1)
}

func TestSiteTemplateSaveOfACleanMessageHasNoWarnings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteTemplatesCreate(context.Background(), &siteapi.SiteCreateEmailTemplateInput{
		Name: "Clean", Body: siteapi.NewOptString(
			`<p style="color:#202124;font-family:Arial, sans-serif">Hi</p><img src="https://cdn.example.com/a.png" alt="A">`),
	}, siteapi.SiteTemplatesCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", created)
	assert.Empty(t, res.Warnings)
}
