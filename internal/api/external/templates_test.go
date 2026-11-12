package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func entityIDString(id int64) externalapi.EntityId {
	return externalapi.EntityId(id)
}

func TestExternalTemplatesCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeTemplatesRead, apitokens.ScopeTemplatesWrite)
	ctx := context.Background()

	page, err := c.TemplatesList(ctx, externalapi.TemplatesListParams{})
	require.NoError(t, err)
	names := map[string]bool{}
	for _, it := range page.Items {
		names[it.Name] = true
	}
	assert.True(t, names["Welcome"], "fixture template 1 is listed")

	tpl, err := c.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: entityIDString(fixtures.TemplateWelcomeID)})
	require.NoError(t, err)
	assert.Equal(t, "Welcome", tpl.Name)
	assert.Contains(t, tpl.Body, "<p>")

	res, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name:    "Win-back",
		Subject: externalapi.NewOptString("We miss you"),
	})
	require.NoError(t, err)
	assert.Equal(t, "We miss you", res.Subject)

	updated, err := c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{Revision: res.Revision, Name: externalapi.NewOptName("Win-back v2")},
		externalapi.TemplatesUpdateParams{ID: res.ID})
	require.NoError(t, err)
	assert.Equal(t, "Win-back v2", updated.Name)
	assert.Equal(t, "We miss you", updated.Subject, "unset fields are kept")

	err = c.TemplatesDelete(ctx, externalapi.TemplatesDeleteParams{ID: res.ID})
	require.NoError(t, err)

	_, err = c.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: res.ID})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

func TestExternalTemplatesScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := env.ExternalAnonymous(t).TemplatesList(ctx, externalapi.TemplatesListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)

	// A contacts-only token reaches no template operation.
	other := env.ExternalScoped(t, apitokens.ScopeContactsRead, apitokens.ScopeContactsWrite)
	_, err = other.TemplatesList(ctx, externalapi.TemplatesListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	// Read-only token cannot write.
	ro := env.ExternalScoped(t, apitokens.ScopeTemplatesRead)
	_, err = ro.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: entityIDString(fixtures.TemplateWelcomeID)})
	require.NoError(t, err)

	_, err = ro.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{Name: "x"})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = ro.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{Revision: 1}, externalapi.TemplatesUpdateParams{ID: entityIDString(fixtures.TemplateWelcomeID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	err = ro.TemplatesDelete(ctx, externalapi.TemplatesDeleteParams{ID: entityIDString(fixtures.TemplateWelcomeID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

func TestExternalTemplatesAreWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	id := entityIDString(fixtures.TemplateGlobexID)

	c := env.ExternalScoped(t, apitokens.ScopeTemplatesRead, apitokens.ScopeTemplatesWrite)

	list, err := c.TemplatesList(ctx, externalapi.TemplatesListParams{})
	require.NoError(t, err)
	for _, it := range list.Items {
		assert.NotEqual(t, fixtures.TemplateGlobexName, it.Name)
	}
	_, err = c.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	_, err = c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{Revision: 1, Name: externalapi.NewOptName("hijack")}, externalapi.TemplatesUpdateParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	err = c.TemplatesDelete(ctx, externalapi.TemplatesDeleteParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}
