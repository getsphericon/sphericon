package contacts_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestCustomFieldsListTheWorkspaceCatalogueAscendingByKey(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)

	fields, err := contacts.CustomFields(context.Background(), s)
	require.NoError(t, err)

	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.Key
	}
	assert.NotEmpty(t, keys)
	assert.True(t, slices.IsSorted(keys), "keys %v are not ascending", keys)
	assert.NotContains(t, keys, fixtures.CustomFieldGlobexKey)
}

func TestGetLoadsTheContactTagsSortedByName(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	s := env.DB.Scoped(fixtures.AcmeID)

	c, err := m.Get(context.Background(), s, fixtures.ContactAliceID)
	require.NoError(t, err)

	names := make([]string, len(c.Edges.Tags))
	for i, tg := range c.Edges.Tags {
		names[i] = tg.Name
	}
	assert.Equal(t, []string{fixtures.TagNewsletterName, fixtures.TagVipName}, names)
}

func TestGetReportsAContactOfAnotherWorkspaceAsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	other := env.DB.Scoped(fixtures.GlobexID)

	_, err := m.Get(context.Background(), other, fixtures.ContactAliceID)
	assert.ErrorIs(t, err, contacts.ErrNotFound)
}
