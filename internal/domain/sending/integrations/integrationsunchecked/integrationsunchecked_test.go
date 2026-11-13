package integrationsunchecked_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations/integrationsunchecked"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// deletion opens the Deletion of the Integration id under Acme's lock and runs then on it, in the
// transaction that holds the lock and queues its jobs.
func deletion(t *testing.T, env *testhelper.TestEnv, id int64, then func(ctx context.Context, d *integrationsunchecked.Deletion, q events.Enqueuer) error) error {
	t.Helper()
	ctx := t.Context()
	return env.Bus.WithinWorkspaceLockEnqueueTx(ctx, env.DB.Scoped(fixtures.AcmeID), func(wl *ent.WorkspaceLocked, _ events.Publisher, q events.Enqueuer) error {
		d, err := integrationsunchecked.DeletionIn(ctx, wl, id)
		if err != nil {
			return err
		}
		return then(ctx, d, q)
	})
}

// apply applies the deletion it is handed at now.
func apply(ctx context.Context, d *integrationsunchecked.Deletion, q events.Enqueuer) error {
	return d.Apply(ctx, time.Now(), q)
}

// integration reads Acme's Integration id as it is now, deleted or not.
func integration(t *testing.T, env *testhelper.TestEnv, id int64) *ent.Integration {
	t.Helper()
	row, err := env.DB.Scoped(fixtures.AcmeID).Integration().Get(t.Context(), id)
	require.NoError(t, err)
	return row
}

// releases is the webhook releases queued, as their job's args.
func releases(t *testing.T, env *testhelper.TestEnv) []integrations.ReleaseWebhookArgs {
	t.Helper()
	queued := env.JobsOf(t, integrations.ReleaseWebhookArgs{}.Kind())
	out := make([]integrations.ReleaseWebhookArgs, 0, len(queued))
	for _, args := range queued {
		encoded, err := json.Marshal(args)
		require.NoError(t, err)
		var release integrations.ReleaseWebhookArgs
		require.NoError(t, json.Unmarshal(encoded, &release))
		out = append(out, release)
	}
	return out
}

// applyThenRefuse applies the deletion it is handed, then fails its transaction with errRefused,
// as a refusal after the write would.
func applyThenRefuse(ctx context.Context, d *integrationsunchecked.Deletion, q events.Enqueuer) error {
	return errors.Join(apply(ctx, d, q), errRefused)
}

var errRefused = errors.New("refused after the apply")

// A deleted Integration gives up its Channel's default in the statement that discards it.
func TestApplyingTheDeletionOfAKeptIntegrationDiscardsItAndGivesUpTheDefault(t *testing.T) {
	env := testhelper.Setup(t)
	var done bool

	err := deletion(t, env, fixtures.IntegrationAcmeDefaultID, func(ctx context.Context, d *integrationsunchecked.Deletion, q events.Enqueuer) error {
		done = d.Done()
		return apply(ctx, d, q)
	})

	require.NoError(t, err)
	assert.False(t, done)
	row := integration(t, env, fixtures.IntegrationAcmeDefaultID)
	assert.NotNil(t, row.DiscardedAt)
	assert.False(t, row.IsDefault)
	queued := releases(t, env)
	assert.Empty(t, queued, "an email Integration holds no webhook")
}

// Deleting a deleted Integration changes nothing: the deletion reports it done, and applying it
// writes and queues nothing.
func TestTheDeletionOfADeletedIntegrationIsDone(t *testing.T) {
	env := testhelper.Setup(t)
	before := integration(t, env, fixtures.IntegrationAcmeArchivedID)
	var done bool

	err := deletion(t, env, fixtures.IntegrationAcmeArchivedID, func(ctx context.Context, d *integrationsunchecked.Deletion, q events.Enqueuer) error {
		done = d.Done()
		return apply(ctx, d, q)
	})

	require.NoError(t, err)
	assert.True(t, done)
	after := integration(t, env, fixtures.IntegrationAcmeArchivedID)
	assert.Equal(t, before.DiscardedAt.UTC(), after.DiscardedAt.UTC(), "the first deletion's moment stays")
	assert.Equal(t, before.Revision, after.Revision)
	queued := releases(t, env)
	assert.Empty(t, queued)
}

// The bot of a deleted Telegram Integration lets go of its webhook through a job queued in the
// deletion's transaction: only when it held the webhook (enabled), and only if the deletion commits.
func TestTheDeletionOfATelegramIntegrationQueuesTheReleaseOfItsWebhook(t *testing.T) {
	t.Run("an enabled bot", func(t *testing.T) {
		env := testhelper.Setup(t)

		err := deletion(t, env, fixtures.IntegrationAcmeTelegramID, apply)

		require.NoError(t, err)
		queued := releases(t, env)
		want := integrations.ReleaseWebhookArgs{WorkspaceID: fixtures.AcmeID, IntegrationID: fixtures.IntegrationAcmeTelegramID}
		assert.Equal(t, []integrations.ReleaseWebhookArgs{want}, queued)
	})

	t.Run("a disabled bot holds no webhook", func(t *testing.T) {
		env := testhelper.Setup(t)
		env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeTelegramID).SetEnabled(false).ExecX(t.Context())

		err := deletion(t, env, fixtures.IntegrationAcmeTelegramID, apply)

		require.NoError(t, err)
		queued := releases(t, env)
		assert.Empty(t, queued)
	})

	t.Run("a deletion rolled back", func(t *testing.T) {
		env := testhelper.Setup(t)

		err := deletion(t, env, fixtures.IntegrationAcmeTelegramID, applyThenRefuse)

		require.ErrorIs(t, err, errRefused)
		queued := releases(t, env)
		assert.Empty(t, queued)
		kept := integration(t, env, fixtures.IntegrationAcmeTelegramID)
		assert.Nil(t, kept.DiscardedAt)
	})
}

func TestTheDeletionOfAnUnknownIntegrationIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	for name, id := range map[string]int64{"unknown": 999999, "another Workspace's": fixtures.IntegrationGlobexID} {
		t.Run(name, func(t *testing.T) {
			err := deletion(t, env, id, func(context.Context, *integrationsunchecked.Deletion, events.Enqueuer) error { return nil })

			require.ErrorIs(t, err, integrations.ErrNotFound)
		})
	}
}
