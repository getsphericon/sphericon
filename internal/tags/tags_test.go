package tags_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/tags"
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

func TestApplyAutoCreatesOnFirstUseAndIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := tags.New()
	ctx := context.Background()

	before, err := env.DB.Tag.Query().Count(ctx)
	require.NoError(t, err)

	got, err := m.Apply(ctx, acme, fixtures.ContactBobID, "  churn-risk ")
	require.NoError(t, err)
	assert.Equal(t, "churn-risk", got.Name, "names are trimmed")

	again, err := m.Apply(ctx, acme, fixtures.ContactBobID, "churn-risk")
	require.NoError(t, err)
	assert.Equal(t, got.ID, again.ID, "applying twice reuses the one Tag")

	after, err := env.DB.Tag.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before+1, after)

	has, err := m.ForContact(ctx, acme, fixtures.ContactBobID)
	require.NoError(t, err)
	assert.Equal(t, []string{"churn-risk"}, names(has))

	// An existing Tag is reused, not duplicated.
	vip, err := m.Apply(ctx, acme, fixtures.ContactBobID, fixtures.TagVipName)
	require.NoError(t, err)
	assert.Equal(t, int64(fixtures.TagVipID), vip.ID)
}

func TestApplyRejectsBlankNameAndUnknownContact(t *testing.T) {
	env := testhelper.Setup(t)
	acme, globex := env.DB.Scoped(fixtures.AcmeID), env.DB.Scoped(fixtures.GlobexID)
	m := tags.New()
	ctx := context.Background()

	_, err := m.Apply(ctx, acme, fixtures.ContactAliceID, "   ")
	require.ErrorIs(t, err, tags.ErrInvalidName)

	_, err = m.Apply(ctx, acme, 999999, "x")
	require.ErrorIs(t, err, tags.ErrContactNotFound)
	_, err = m.Apply(ctx, globex, fixtures.ContactAliceID, "x")
	require.ErrorIs(t, err, tags.ErrContactNotFound)

	n, err := env.DB.Tag.Query().Where(tag.Name("x")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a failed apply creates no Tag")
}

func TestRemoveDetachesAndKeepsTheTag(t *testing.T) {
	env := testhelper.Setup(t)
	acme, globex := env.DB.Scoped(fixtures.AcmeID), env.DB.Scoped(fixtures.GlobexID)
	m := tags.New()
	ctx := context.Background()

	require.NoError(t, m.Remove(ctx, acme, fixtures.ContactAliceID, fixtures.TagVipName))
	got, err := m.ForContact(ctx, acme, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Equal(t, []string{"newsletter"}, names(got))

	// The Tag stays in the catalogue, and contact 3 keeps it.
	exists, err := env.DB.Tag.Query().Where(tag.Name(fixtures.TagVipName)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, exists)
	c3, err := m.ForContact(ctx, acme, fixtures.ContactCarolID)
	require.NoError(t, err)
	assert.Equal(t, []string{fixtures.TagVipName}, names(c3))

	// Idempotent: removing an absent or unknown tag succeeds.
	require.NoError(t, m.Remove(ctx, acme, fixtures.ContactAliceID, fixtures.TagVipName))
	require.NoError(t, m.Remove(ctx, acme, fixtures.ContactAliceID, "never-existed"))

	require.ErrorIs(t, m.Remove(ctx, acme, 999999, fixtures.TagVipName), tags.ErrContactNotFound)
	require.ErrorIs(t, m.Remove(ctx, globex, fixtures.ContactCarolID, fixtures.TagVipName), tags.ErrContactNotFound)
}

func TestSetMakesTheNamedTagsTheContactsCompleteSet(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := tags.New()
	ctx := context.Background()

	// Alice has vip and newsletter: keep vip, drop newsletter, create a new tag; blank and repeated names change nothing.
	require.NoError(t, m.Set(ctx, acme, fixtures.ContactAliceID, []string{fixtures.TagVipName, " fresh ", "fresh", " "}))
	got, err := m.ForContact(ctx, acme, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Equal(t, []string{"fresh", fixtures.TagVipName}, names(got))

	require.NoError(t, m.Set(ctx, acme, fixtures.ContactAliceID, []string{}))
	got, err = m.ForContact(ctx, acme, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Empty(t, got)

	require.ErrorIs(t, m.Set(ctx, acme, 999999, []string{"x"}), tags.ErrContactNotFound)
}

func TestForeignContactIsRefusedOnEveryOperation(t *testing.T) {
	env := testhelper.Setup(t)
	acme, globex := env.DB.Scoped(fixtures.AcmeID), env.DB.Scoped(fixtures.GlobexID)
	m := tags.New()
	ctx := context.Background()

	_, err := m.Apply(ctx, acme, fixtures.ContactGlobexID, "vip")
	require.ErrorIs(t, err, tags.ErrContactNotFound)
	_, err = m.ForContact(ctx, acme, fixtures.ContactGlobexID)
	require.ErrorIs(t, err, tags.ErrContactNotFound)
	require.ErrorIs(t, m.Remove(ctx, acme, fixtures.ContactGlobexID, fixtures.TagGlobexName), tags.ErrContactNotFound)

	// The foreign contact was left untouched.
	got, err := m.ForContact(ctx, globex, fixtures.ContactGlobexID)
	require.NoError(t, err)
	assert.Empty(t, got)
}
