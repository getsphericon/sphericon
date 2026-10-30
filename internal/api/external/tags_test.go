package external_test

import (
	"context"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tagNames(items []externalapi.TagResource) []string {
	return lo.Map(items, func(t externalapi.TagResource, _ int) string { return t.Name })
}

// Fixtures: tag "vip" on contacts 1 and 3, "newsletter" on contact 1, "unused" on
// nobody; contact 2 has none.
func TestExternalTagsListAndPerContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read")
	ctx := context.Background()

	all, err := c.TagsList(ctx, externalapi.TagsListParams{})
	require.NoError(t, err)
	listed, ok := all.(*externalapi.TagsListOK)
	require.Truef(t, ok, "got %T", all)
	assert.Equal(t, []string{"newsletter", "renewal-due", "unused", "vip"}, tagNames(listed.Items))

	mine, err := c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
	contactTags, ok := mine.(*externalapi.TagsListForContactOK)
	require.Truef(t, ok, "got %T", mine)
	assert.Equal(t, []string{"newsletter", "vip"}, tagNames(contactTags.Items))

	missing, err := c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsListForContactNotFound{}, missing)
}

func TestExternalTagsApplyAndRemove(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read", "contacts:write")
	ctx := context.Background()

	// First use creates the Tag and applies it.
	applied, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "churn-risk"}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	tag, ok := applied.(*externalapi.TagResource)
	require.Truef(t, ok, "got %T", applied)
	assert.Equal(t, "churn-risk", tag.Name)

	mine, err := c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Equal(t, []string{"churn-risk"}, tagNames(mine.(*externalapi.TagsListForContactOK).Items))

	// The new Tag is in the workspace catalogue.
	all, err := c.TagsList(ctx, externalapi.TagsListParams{})
	require.NoError(t, err)
	assert.Contains(t, tagNames(all.(*externalapi.TagsListOK).Items), "churn-risk")

	removed, err := c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: entityIDString(fixtures.ContactBobID), Name: "churn-risk"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsRemoveNoContent{}, removed)

	mine, err = c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Empty(t, mine.(*externalapi.TagsListForContactOK).Items)

	// Names with spaces and slashes survive the path round trip.
	_, err = c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "plan / pro"}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	removed, err = c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: entityIDString(fixtures.ContactBobID), Name: "plan / pro"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsRemoveNoContent{}, removed)
	mine, err = c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Empty(t, mine.(*externalapi.TagsListForContactOK).Items)

	// Unknown contact and blank name.
	nf, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "x"}, externalapi.TagsApplyParams{ContactId: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsApplyNotFound{}, nf)
	blank, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "  "}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsApplyUnprocessableEntity{}, blank)
}

func TestExternalTagsRequireScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	readOnly := env.ExternalScoped(t, "contacts:read")
	denied, err := readOnly.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "x"}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsApplyForbidden{}, denied)
	deniedRemove, err := readOnly.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: entityIDString(fixtures.ContactAliceID), Name: "vip"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsRemoveForbidden{}, deniedRemove)

	writeOnly := env.ExternalScoped(t, "contacts:write")
	deniedList, err := writeOnly.TagsList(ctx, externalapi.TagsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsListForbidden{}, deniedList)
}

func TestExternalTagsApplyRefusesAForeignContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read", "contacts:write")
	ctx := context.Background()

	got, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "brand-new"}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsApplyNotFound{}, got)

	all, err := c.TagsList(ctx, externalapi.TagsListParams{})
	require.NoError(t, err)
	assert.NotContains(t, tagNames(all.(*externalapi.TagsListOK).Items), "brand-new", "a refused apply creates no Tag")
}
