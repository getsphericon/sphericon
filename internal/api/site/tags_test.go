package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func siteTagNames(items []siteapi.SiteTagResource) []string {
	return lo.Map(items, func(t siteapi.SiteTagResource, _ int) string { return t.Name })
}

func TestSiteTagsRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteTagsList(context.Background(), siteapi.SiteTagsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

// Fixtures: workspace "acme" has tags vip (contacts 1, 3), newsletter (contact 1)
// and unused; contact 2 has none.
func TestSiteTagsListApplyRemove(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	listed, err := c.SiteTagsList(ctx, siteapi.SiteTagsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Equal(t, []string{"newsletter", "renewal-due", "unused", "vip"}, siteTagNames(listed.Items))

	mine, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: fixtures.AcmePublicID, ContactId: idStr(fixtures.ContactAliceID)})
	require.NoError(t, err)
	assert.Equal(t, []string{"newsletter", "vip"}, siteTagNames(mine.Items))

	tag, err := c.SiteTagsApply(ctx, &siteapi.ApplyTagInput{Name: "churn-risk"}, siteapi.SiteTagsApplyParams{WorkspaceId: fixtures.AcmePublicID, ContactId: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Equal(t, "churn-risk", tag.Name)

	mine, err = c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: fixtures.AcmePublicID, ContactId: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Equal(t, []string{"churn-risk"}, siteTagNames(mine.Items))

	err = c.SiteTagsRemove(ctx, siteapi.SiteTagsRemoveParams{WorkspaceId: fixtures.AcmePublicID, ContactId: idStr(fixtures.ContactBobID), Name: "churn-risk"})
	require.NoError(t, err)
	mine, err = c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: fixtures.AcmePublicID, ContactId: idStr(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Empty(t, mine.Items)

	// Errors: unknown contact, blank name, unknown workspace.
	_, err = c.SiteTagsApply(ctx, &siteapi.ApplyTagInput{Name: "x"}, siteapi.SiteTagsApplyParams{WorkspaceId: fixtures.AcmePublicID, ContactId: 999999})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteTagsApply(ctx, &siteapi.ApplyTagInput{Name: " "}, siteapi.SiteTagsApplyParams{WorkspaceId: fixtures.AcmePublicID, ContactId: idStr(fixtures.ContactBobID)})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	_, err = c.SiteTagsList(ctx, siteapi.SiteTagsListParams{WorkspaceId: "NoSuchWs"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

func TestSiteTagsApplyRefusesAForeignContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := c.SiteTagsApply(ctx, &siteapi.ApplyTagInput{Name: "brand-new"}, siteapi.SiteTagsApplyParams{WorkspaceId: fixtures.AcmePublicID, ContactId: idStr(fixtures.ContactGlobexID)})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	all, err := c.SiteTagsList(ctx, siteapi.SiteTagsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.NotContains(t, siteTagNames(all.Items), "brand-new", "a refused apply creates no Tag")
}
