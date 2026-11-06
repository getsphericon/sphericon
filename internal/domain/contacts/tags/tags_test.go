package tags_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures (workspace 1): tag 1 "vip" on contacts 1 and 3; tag 2 "newsletter" on
// contact 1; tag 3 "unused" on nobody. Contact 2 has no tags.
func names(ts []*ent.Tag) []string {
	return lo.Map(ts, func(t *ent.Tag, _ int) string { return t.Name })
}

func TestListReturnsTheWorkspaceCatalogue(t *testing.T) {
	env := testhelper.Setup(t)
	acme, globex := env.DB.Scoped(fixtures.AcmeID), env.DB.Scoped(fixtures.GlobexID)
	m := tags.New()

	got, err := m.List(context.Background(), acme)
	require.NoError(t, err)
	assert.Equal(t, []string{"newsletter", fixtures.TagRenewalDueName, "unused", fixtures.TagVipName}, names(got))

	other, err := m.List(context.Background(), globex)
	require.NoError(t, err)
	assert.Equal(t, []string{fixtures.TagGlobexName}, names(other), "each workspace sees only its own tags")
}

func TestForContactListsOnlyItsTags(t *testing.T) {
	env := testhelper.Setup(t)
	acme, globex := env.DB.Scoped(fixtures.AcmeID), env.DB.Scoped(fixtures.GlobexID)
	m := tags.New()
	ctx := context.Background()

	got, err := m.ForContact(ctx, acme, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Equal(t, []string{"newsletter", fixtures.TagVipName}, names(got))

	none, err := m.ForContact(ctx, acme, fixtures.ContactBobID)
	require.NoError(t, err)
	assert.Empty(t, none)

	_, err = m.ForContact(ctx, acme, 999999)
	require.ErrorIs(t, err, tags.ErrContactNotFound)
	_, err = m.ForContact(ctx, globex, fixtures.ContactAliceID)
	require.ErrorIs(t, err, tags.ErrContactNotFound, "a contact of another workspace is invisible")
}
