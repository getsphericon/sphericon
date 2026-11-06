package contacts_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixtures (workspace 1): tag 1 "vip" on contacts 1 and 3; tag 2 "newsletter" on
// contact 1; tag 3 "unused" on nobody. Contact 2 has no tags.
func catalogueNames(ts []*ent.Tag) []string {
	return lo.Map(ts, func(t *ent.Tag, _ int) string { return t.Name })
}

func TestApplyAutoCreatesOnFirstUseAndIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m, c := tags.New(), contacts.New(env.Bus)
	ctx := context.Background()

	before, err := env.DB.Tag.Query().Count(ctx)
	require.NoError(t, err)

	got, err := c.ApplyTag(ctx, acme, fixtures.ContactBobID, "  churn-risk ")
	require.NoError(t, err)
	assert.Equal(t, "churn-risk", got.Name, "names are trimmed")

	again, err := c.ApplyTag(ctx, acme, fixtures.ContactBobID, "churn-risk")
	require.NoError(t, err)
	assert.Equal(t, got.ID, again.ID, "applying twice reuses the one Tag")

	after, err := env.DB.Tag.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before+1, after)

	has, err := m.ForContact(ctx, acme, fixtures.ContactBobID)
	require.NoError(t, err)
	assert.Equal(t, []string{"churn-risk"}, catalogueNames(has))

	// An existing Tag is reused, not duplicated.
	vip, err := c.ApplyTag(ctx, acme, fixtures.ContactBobID, fixtures.TagVipName)
	require.NoError(t, err)
	assert.Equal(t, int64(fixtures.TagVipID), vip.ID)
}

func TestApplyRejectsBlankNameAndUnknownContact(t *testing.T) {
	env := testhelper.Setup(t)
	acme, globex := env.DB.Scoped(fixtures.AcmeID), env.DB.Scoped(fixtures.GlobexID)
	c := contacts.New(env.Bus)
	ctx := context.Background()

	_, err := c.ApplyTag(ctx, acme, fixtures.ContactAliceID, "   ")
	require.ErrorIs(t, err, tags.ErrInvalidName)

	_, err = c.ApplyTag(ctx, acme, 999999, "x")
	require.ErrorIs(t, err, tags.ErrContactNotFound)
	_, err = c.ApplyTag(ctx, globex, fixtures.ContactAliceID, "x")
	require.ErrorIs(t, err, tags.ErrContactNotFound)

	n, err := env.DB.Tag.Query().Where(tag.Name("x")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a failed apply creates no Tag")
}

func TestRemoveDetachesAndKeepsTheTag(t *testing.T) {
	env := testhelper.Setup(t)
	acme, globex := env.DB.Scoped(fixtures.AcmeID), env.DB.Scoped(fixtures.GlobexID)
	m, c := tags.New(), contacts.New(env.Bus)
	ctx := context.Background()

	require.NoError(t, c.RemoveTag(ctx, acme, fixtures.ContactAliceID, fixtures.TagVipName))
	got, err := m.ForContact(ctx, acme, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Equal(t, []string{"newsletter"}, catalogueNames(got))

	// The Tag stays in the catalogue, and contact 3 keeps it.
	exists, err := env.DB.Tag.Query().Where(tag.Name(fixtures.TagVipName)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, exists)
	c3, err := m.ForContact(ctx, acme, fixtures.ContactCarolID)
	require.NoError(t, err)
	assert.Equal(t, []string{fixtures.TagVipName}, catalogueNames(c3))

	// Idempotent: removing an absent or unknown tag succeeds.
	require.NoError(t, c.RemoveTag(ctx, acme, fixtures.ContactAliceID, fixtures.TagVipName))
	require.NoError(t, c.RemoveTag(ctx, acme, fixtures.ContactAliceID, "never-existed"))

	require.ErrorIs(t, c.RemoveTag(ctx, acme, 999999, fixtures.TagVipName), tags.ErrContactNotFound)
	require.ErrorIs(t, c.RemoveTag(ctx, globex, fixtures.ContactCarolID, fixtures.TagVipName), tags.ErrContactNotFound)
}

func TestSetMakesTheNamedTagsTheContactsCompleteSet(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := tags.New()
	ctx := context.Background()
	set := func(contactID int64, names []string) error {
		return env.Bus.WithinScopedTx(ctx, acme, func(ts *ent.Scoped, pub events.Publisher) error {
			return contacts.SetTagsIn(ctx, ts, pub, contactID, names)
		})
	}

	// Alice has vip and newsletter: keep vip, drop newsletter, create a new tag; blank and repeated names change nothing.
	require.NoError(t, set(fixtures.ContactAliceID, []string{fixtures.TagVipName, " fresh ", "fresh", " "}))
	got, err := m.ForContact(ctx, acme, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Equal(t, []string{"fresh", fixtures.TagVipName}, catalogueNames(got))

	require.NoError(t, set(fixtures.ContactAliceID, []string{}))
	got, err = m.ForContact(ctx, acme, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Empty(t, got)

	require.ErrorIs(t, set(999999, []string{"x"}), tags.ErrContactNotFound)
}

func TestForeignContactIsRefusedOnEveryOperation(t *testing.T) {
	env := testhelper.Setup(t)
	acme, globex := env.DB.Scoped(fixtures.AcmeID), env.DB.Scoped(fixtures.GlobexID)
	m, c := tags.New(), contacts.New(env.Bus)
	ctx := context.Background()

	_, err := c.ApplyTag(ctx, acme, fixtures.ContactGlobexID, "vip")
	require.ErrorIs(t, err, tags.ErrContactNotFound)
	_, err = m.ForContact(ctx, acme, fixtures.ContactGlobexID)
	require.ErrorIs(t, err, tags.ErrContactNotFound)
	require.ErrorIs(t, c.RemoveTag(ctx, acme, fixtures.ContactGlobexID, fixtures.TagGlobexName), tags.ErrContactNotFound)

	// The foreign contact was left untouched.
	got, err := m.ForContact(ctx, globex, fixtures.ContactGlobexID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// Applying to a contact of another workspace is refused inside the transaction and
// rolls back: the tag is not auto-created.
func TestApplyToAnotherWorkspacesContactCreatesNoTag(t *testing.T) {
	env := testhelper.Setup(t)
	m, c := tags.New(), contacts.New(env.Bus)
	ctx := context.Background()
	otherWsID := env.DB.Scoped(fixtures.GlobexID)

	_, err := c.ApplyTag(ctx, otherWsID, fixtures.ContactAliceID, "sneaky")
	require.ErrorIs(t, err, tags.ErrContactNotFound)

	got, err := m.List(ctx, otherWsID)
	require.NoError(t, err)
	for _, tg := range got {
		assert.NotEqual(t, "sneaky", tg.Name)
	}
	err = c.RemoveTag(ctx, otherWsID, fixtures.ContactAliceID, fixtures.TagVipName)
	assert.ErrorIs(t, err, tags.ErrContactNotFound)
}
