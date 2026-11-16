package tagsunchecked_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags/tagsunchecked"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// deletion opens the Deletion of the Tag id under Acme's lock and runs then on it, in the
// transaction that holds the lock.
func deletion(t *testing.T, env *testhelper.TestEnv, id int64, then func(ctx context.Context, l *ent.WorkspaceLocked, d *tagsunchecked.Deletion) error) error {
	t.Helper()
	ctx := t.Context()
	return env.Bus.WithinWorkspaceLockTx(ctx, env.DB.Scoped(fixtures.AcmeID), func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		d, err := unchecked.DeletionIn(ctx, wl, id)
		if err != nil {
			return err
		}
		return then(ctx, wl, d)
	})
}

var unchecked = tagsunchecked.New(tags.New())

func TestApplyingTheDeletionOfATagRemovesItFromTheCatalogue(t *testing.T) {
	env := testhelper.Setup(t)
	var name string

	err := deletion(t, env, fixtures.TagUnusedID, func(ctx context.Context, l *ent.WorkspaceLocked, d *tagsunchecked.Deletion) error {
		name = d.Name()
		return unchecked.ApplyDeletion(ctx, l, d)
	})

	require.NoError(t, err)
	assert.Equal(t, fixtures.TagUnusedName, name, "the Removal check reads the name the deletion opened")
	_, err = tags.New().Get(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.TagUnusedID)
	require.ErrorIs(t, err, tags.ErrNotFound)
}

func TestTheDeletionOfAnUnknownTagIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	for name, id := range map[string]int64{"unknown": 999999, "another Workspace's": fixtures.TagGlobexID} {
		t.Run(name, func(t *testing.T) {
			err := deletion(t, env, id, func(context.Context, *ent.WorkspaceLocked, *tagsunchecked.Deletion) error { return nil })

			require.ErrorIs(t, err, tags.ErrNotFound)
		})
	}
}
