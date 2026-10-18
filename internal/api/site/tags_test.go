package site_test

import (
	"context"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func siteTagNames(items []siteapi.SiteTagResource) []string {
	return lo.Map(items, func(t siteapi.SiteTagResource, _ int) string { return t.Name })
}

func TestSiteTagsRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteTagsList(context.Background(), siteapi.SiteTagsListParams{Slug: fixtures.AcmeSlug})
	require.Error(t, err)
}

// Fixtures: workspace "acme" has tags vip (contacts 1, 3), newsletter (contact 1)
// and unused; contact 2 has none.
func TestSiteTagsListApplyRemove(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	all, err := c.SiteTagsList(ctx, siteapi.SiteTagsListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	listed, ok := all.(*siteapi.SiteTagsListOK)
	require.Truef(t, ok, "got %T", all)
	assert.Equal(t, []string{"newsletter", "unused", "vip"}, siteTagNames(listed.Items))

	mine, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{Slug: fixtures.AcmeSlug, ContactId: idStr(fixtures.ContactAliceID)})
	require.NoError(t, err)
	assert.Equal(t, []string{"newsletter", "vip"}, siteTagNames(mine.(*siteapi.SiteTagsListForContactOK).Items))

	applied, err := c.SiteTagsApply(ctx, &siteapi.SiteApplyTagInput{Name: "churn-risk"}, siteapi.SiteTagsApplyParams{Slug: fixtures.AcmeSlug, ContactId: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	tag, ok := applied.(*siteapi.SiteTagResource)
	require.Truef(t, ok, "got %T", applied)
	assert.Equal(t, "churn-risk", tag.Name)

	mine, err = c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{Slug: fixtures.AcmeSlug, ContactId: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Equal(t, []string{"churn-risk"}, siteTagNames(mine.(*siteapi.SiteTagsListForContactOK).Items))

	removed, err := c.SiteTagsRemove(ctx, siteapi.SiteTagsRemoveParams{Slug: fixtures.AcmeSlug, ContactId: idStr(fixtures.ContactBobID), Name: "churn-risk"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsRemoveNoContent{}, removed)
	mine, err = c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{Slug: fixtures.AcmeSlug, ContactId: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Empty(t, mine.(*siteapi.SiteTagsListForContactOK).Items)

	// Errors: unknown contact, blank name, unknown workspace.
	nf, err := c.SiteTagsApply(ctx, &siteapi.SiteApplyTagInput{Name: "x"}, siteapi.SiteTagsApplyParams{Slug: fixtures.AcmeSlug, ContactId: 999999})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsApplyNotFound{}, nf)
	blank, err := c.SiteTagsApply(ctx, &siteapi.SiteApplyTagInput{Name: " "}, siteapi.SiteTagsApplyParams{Slug: fixtures.AcmeSlug, ContactId: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsApplyUnprocessableEntity{}, blank)
	ws, err := c.SiteTagsList(ctx, siteapi.SiteTagsListParams{Slug: "does-not-exist"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsListNotFound{}, ws)
}

func TestSiteTagsApplyRefusesAForeignContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	got, err := c.SiteTagsApply(ctx, &siteapi.SiteApplyTagInput{Name: "brand-new"}, siteapi.SiteTagsApplyParams{Slug: fixtures.AcmeSlug, ContactId: idStr(fixtures.ContactGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsApplyNotFound{}, got)

	all, err := c.SiteTagsList(ctx, siteapi.SiteTagsListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.NotContains(t, siteTagNames(all.(*siteapi.SiteTagsListOK).Items), "brand-new", "a refused apply creates no Tag")
}
