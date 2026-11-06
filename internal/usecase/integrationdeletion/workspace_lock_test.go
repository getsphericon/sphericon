package integrationdeletion_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
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

	errs := testhelper.QueuedBehind(t, r.sqlDB, testhelper.HoldWorkspaceLock(t, r.bus, r.s), r.remove, r.schedule)

	require.NoError(t, errs["delete"])
	require.ErrorIs(t, errs["schedule"], outboundmessages.ErrRouteHeld)
	assert.Equal(t, broadcast.StatusDraft, r.broadcast(t).Status)
}

func TestAScheduleQueuedFirstRefusesTheDeleteOfTheIntegrationItSendsThrough(t *testing.T) {
	r := newIntegrationRace(t)

	errs := testhelper.QueuedBehind(t, r.sqlDB, testhelper.HoldWorkspaceLock(t, r.bus, r.s), r.schedule, r.remove)

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
	sqlDB            *sql.DB
	bus              *events.Bus
	s                *ent.Scoped
	sends            *broadcasts.Module
	remove, schedule testhelper.Writer
}

func newIntegrationRace(t *testing.T) integrationRace {
	t.Helper()
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	s := entclient.New(sqlDB).Scoped(fixtures.AcmeID)
	bus := events.New(sqlDB, entclient.NewTx)
	bus.UseJobs(testhelper.AcceptingQueue{})
	out := outboundmessages.New(bus, holds.New(func() bool { return true }, time.Now), anySender{}, messagetoken.New("test-secret", "http://local"))
	sends := broadcasts.New(bus, testhelper.Renderer(), out)
	deletion := integrationdeletion.New(bus, integrations.New(bus, nil, nil, nil, nil, ""))

	_, err := sends.Update(ctx, s, fixtures.BroadcastDraftID, broadcasts.UpdateInput{IntegrationID: patch.Set[int64](fixtures.IntegrationAcmeGenericAID)})
	require.NoError(t, err)
	return integrationRace{
		sqlDB: sqlDB,
		bus:   bus,
		s:     s,
		sends: sends,
		remove: testhelper.Writer{Name: "delete", Run: func() error {
			return deletion.Delete(ctx, s, fixtures.IntegrationAcmeGenericAID)
		}},
		schedule: testhelper.Writer{Name: "schedule", Run: func() error {
			_, err := sends.Schedule(ctx, s, fixtures.BroadcastDraftID, time.Now().Add(time.Hour))
			return err
		}},
	}
}

func (r integrationRace) broadcast(t *testing.T) *ent.Broadcast {
	t.Helper()
	b, err := r.sends.Get(context.Background(), r.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	return b
}
