package sendingdomainsunchecked_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains/sendingdomainsunchecked"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// deletion opens the Deletion of the Sending domain id under Acme's lock and runs then on it, in
// the transaction that holds the lock.
func deletion(t *testing.T, env *testhelper.TestEnv, id int64, then func(ctx context.Context, d *sendingdomainsunchecked.Deletion) error) error {
	t.Helper()
	ctx := t.Context()
	return env.Bus.WithinWorkspaceLockTx(ctx, env.DB.Scoped(fixtures.AcmeID), func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		d, err := sendingdomainsunchecked.DeletionIn(ctx, wl, id)
		if err != nil {
			return err
		}
		return then(ctx, d)
	})
}

func TestApplyingTheDeletionOfAKeptSendingDomainDiscardsIt(t *testing.T) {
	env := testhelper.Setup(t)
	var domain string
	var done bool

	err := deletion(t, env, fixtures.SendingDomainVerifiedID, func(ctx context.Context, d *sendingdomainsunchecked.Deletion) error {
		domain, done = d.Domain(), d.Done()
		return d.Apply(ctx, time.Now())
	})

	require.NoError(t, err)
	assert.Equal(t, fixtures.SendingDomainVerifiedDomain, domain, "the Removal check reads the domain the deletion opened")
	assert.False(t, done)
	row, err := env.DB.Scoped(fixtures.AcmeID).SendingDomain().Get(t.Context(), fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.NotNil(t, row.DiscardedAt)
}

// Deleting a deleted Sending domain changes nothing: the deletion reports it done, and applying it
// writes nothing.
func TestTheDeletionOfADeletedSendingDomainIsDone(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	before, err := s.SendingDomain().Get(t.Context(), fixtures.SendingDomainArchivedID)
	require.NoError(t, err)
	var done bool

	err = deletion(t, env, fixtures.SendingDomainArchivedID, func(ctx context.Context, d *sendingdomainsunchecked.Deletion) error {
		done = d.Done()
		return d.Apply(ctx, time.Now())
	})

	require.NoError(t, err)
	assert.True(t, done)
	after, err := s.SendingDomain().Get(t.Context(), fixtures.SendingDomainArchivedID)
	require.NoError(t, err)
	assert.Equal(t, before.DiscardedAt.UTC(), after.DiscardedAt.UTC(), "the first deletion's moment stays")
	assert.Equal(t, before.UpdatedAt.UTC(), after.UpdatedAt.UTC())
}

func TestTheDeletionOfAnUnknownSendingDomainIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	for name, id := range map[string]int64{"unknown": 999999, "another Workspace's": fixtures.SendingDomainGlobexID} {
		t.Run(name, func(t *testing.T) {
			err := deletion(t, env, id, func(context.Context, *sendingdomainsunchecked.Deletion) error { return nil })

			require.ErrorIs(t, err, sendingdomains.ErrNotFound)
		})
	}
}
