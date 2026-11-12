package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func tagNames(items []externalapi.TagResource) []string {
	return lo.Map(items, func(t externalapi.TagResource, _ int) string { return t.Name })
}

// Fixtures: tag "vip" on contacts 1 and 3, "newsletter" on contact 1, "unused" on
// nobody; contact 2 has none.
func TestExternalTagsListAndPerContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsRead)
	ctx := context.Background()

	listed, err := c.TagsList(ctx, externalapi.TagsListParams{})
	require.NoError(t, err)
	assert.Equal(t, []string{"newsletter", "renewal-due", "unused", "vip"}, tagNames(listed.Items))

	contactTags, err := c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
	assert.Equal(t, []string{"newsletter", "vip"}, tagNames(contactTags.Items))

	_, err = c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: 999999})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

func TestExternalTagsApplyAndRemove(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsRead, apitokens.ScopeContactsWrite)
	ctx := context.Background()

	// First use creates the Tag and applies it.
	tag, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "churn-risk"}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Equal(t, "churn-risk", tag.Name)

	mine, err := c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Equal(t, []string{"churn-risk"}, tagNames(mine.Items))

	// The new Tag is in the workspace catalogue.
	all, err := c.TagsList(ctx, externalapi.TagsListParams{})
	require.NoError(t, err)
	assert.Contains(t, tagNames(all.Items), "churn-risk")

	err = c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: entityIDString(fixtures.ContactBobID), Name: "churn-risk"})
	require.NoError(t, err)

	mine, err = c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Empty(t, mine.Items)

	// Names with spaces and slashes survive the path round trip.
	_, err = c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "plan / pro"}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	err = c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: entityIDString(fixtures.ContactBobID), Name: "plan / pro"})
	require.NoError(t, err)
	mine, err = c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: entityIDString(fixtures.ContactBobID)})
	require.NoError(t, err)
	assert.Empty(t, mine.Items)

	// Unknown contact and blank name.
	_, err = c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "x"}, externalapi.TagsApplyParams{ContactId: 999999})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	_, err = c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "  "}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactBobID)})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}

func TestExternalTagsRequireScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	readOnly := env.ExternalScoped(t, apitokens.ScopeContactsRead)
	_, err := readOnly.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "x"}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactAliceID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	err = readOnly.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: entityIDString(fixtures.ContactAliceID), Name: "vip"})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	writeOnly := env.ExternalScoped(t, apitokens.ScopeContactsWrite)
	_, err = writeOnly.TagsList(ctx, externalapi.TagsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

func TestExternalTagsApplyRefusesAForeignContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsRead, apitokens.ScopeContactsWrite)
	ctx := context.Background()

	_, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "brand-new"}, externalapi.TagsApplyParams{ContactId: entityIDString(fixtures.ContactGlobexID)})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	all, err := c.TagsList(ctx, externalapi.TagsListParams{})
	require.NoError(t, err)
	assert.NotContains(t, tagNames(all.Items), "brand-new", "a refused apply creates no Tag")
}
