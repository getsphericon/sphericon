package integrationdeletion_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/integrationdeletion"
)

// anySender resolves every Integration to a sender that accepts every message.
type anySender struct{}

func (anySender) EmailSender(context.Context, *ent.Scoped, *ent.Integration) (messaging.EmailSender, error) {
	return anySender{}, nil
}

func (anySender) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{}, nil
}

// Deleting an Integration and scheduling a draft that sends through it take the Workspace's lock
// (ADR 0099), so the second waits for the first and then reads what it committed: the Integration
// is never deleted under a Broadcast that the check found a draft and that is armed meanwhile.
func TestADeleteOfAnIntegrationQueuedFirstRefusesTheScheduleThroughIt(t *testing.T) {
	r := newIntegrationRace(t)

	errs := r.Queue(t, r.remove, r.schedule)

	require.NoError(t, errs["delete"])
	require.ErrorIs(t, errs["schedule"], outboundmessages.ErrRouteHeld)
	b := r.broadcast(t)
	assert.Equal(t, broadcast.StatusDraft, b.Status)
}

func TestAScheduleQueuedFirstRefusesTheDeleteOfTheIntegrationItSendsThrough(t *testing.T) {
	r := newIntegrationRace(t)

	errs := r.Queue(t, r.schedule, r.remove)

	require.NoError(t, errs["schedule"])
	b := r.broadcast(t)
	var used *liveuse.Error
	require.ErrorAs(t, errs["delete"], &used)
	assert.Equal(t, []liveuse.User{{Kind: liveuse.Broadcast, ID: b.ID, Name: b.Name}}, used.Users)
	assert.Equal(t, broadcast.StatusScheduled, b.Status)
}

// integrationRace is a draft Broadcast that sends through an Integration, with a writer that
// deletes the Integration and one that schedules the Broadcast, on a database of its own.
type integrationRace struct {
	*testhelper.WorkspaceRace

	sends            *broadcasts.Module
	remove, schedule testhelper.Writer
}

func newIntegrationRace(t *testing.T) integrationRace {
	t.Helper()
	ctx := context.Background()
	race := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	s, bus := race.Scope, race.Bus
	out := outboundmessages.New(bus, holds.New(func() bool { return true }, time.Now), anySender{}, messagetoken.New("test-secret", "http://local", time.Now), time.Now)
	sends := broadcasts.New(bus, testhelper.Renderer(), out)
	check := removalcheck.New(messaging.NewResolver(testhelper.Cipher(t), registry.Default()))
	deletion := integrationdeletion.New(bus, integrations.New(bus, nil, nil, nil, "", time.Now), check, time.Now)

	_, err := sends.Update(ctx, s, fixtures.BroadcastDraftID, broadcasts.UpdateInput{Revision: testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastDraftID), IntegrationID: patch.Set[int64](fixtures.IntegrationAcmeGenericAID)})
	require.NoError(t, err)
	return integrationRace{
		WorkspaceRace: race,
		sends:         sends,
		remove: testhelper.Writer{Name: "delete", Run: func() error {
			return deletion.Delete(ctx, s, fixtures.IntegrationAcmeGenericAID)
		}},
		schedule: testhelper.Writer{Name: "schedule", Run: func() error {
			_, err := sends.Schedule(ctx, s, fixtures.BroadcastDraftID, testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastDraftID), time.Now().Add(time.Hour))
			return err
		}},
	}
}

func (r integrationRace) broadcast(t *testing.T) *ent.Broadcast {
	t.Helper()
	b, err := r.sends.Get(context.Background(), r.Scope, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	return b
}

// Promoting an Integration to its Channel's default moves Live use (ADR 0108), so it takes the
// Workspace's lock: a promotion queued behind the Integration's deletion waits for it, then finds
// the Integration gone, and the Channel keeps the default it had.
func TestAPromotionQueuedBehindTheDeleteOfItsIntegrationLeavesTheChannelItsDefault(t *testing.T) {
	ctx := context.Background()
	race := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	s, bus := race.Scope, race.Bus
	presets, err := preset.Load(nil)
	require.NoError(t, err)
	integs := integrations.New(bus, testhelper.Cipher(t), nil, presets, "", time.Now)
	check := removalcheck.New(messaging.NewResolver(testhelper.Cipher(t), registry.Default()))
	deletion := integrationdeletion.New(bus, integs, check, time.Now)

	errs := race.Queue(t,
		testhelper.Writer{Name: "delete", Run: func() error {
			return deletion.Delete(ctx, s, fixtures.IntegrationAcmeSesID)
		}},
		testhelper.Writer{Name: "promote", Run: func() error {
			_, err := integs.Update(ctx, s, fixtures.IntegrationAcmeSesID, integrations.UpdateInput{IsDefault: patch.Set(true)})
			return err
		}},
	)

	require.NoError(t, errs["delete"])
	require.ErrorIs(t, errs["promote"], integrations.ErrNotFound)
	deleted, err := integs.Get(ctx, s, fixtures.IntegrationAcmeSesID)
	require.NoError(t, err)
	assert.NotNil(t, deleted.DiscardedAt)
	assert.False(t, deleted.IsDefault)
	kept, err := integs.Get(ctx, s, fixtures.IntegrationAcmeDefaultID)
	require.NoError(t, err)
	assert.True(t, kept.IsDefault)
}
