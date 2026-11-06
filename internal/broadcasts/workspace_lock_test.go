package broadcasts_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/usecase/tagdeletion"
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
	require.ErrorIs(t, errs["schedule"], outbound.ErrRouteHeld)
	assert.Equal(t, broadcast.StatusDraft, r.broadcast(t).Status)
}

func TestAScheduleQueuedFirstRefusesTheDeleteOfTheIntegrationItSendsThrough(t *testing.T) {
	r := newIntegrationRace(t)

	errs := testhelper.QueuedBehind(t, r.sqlDB, testhelper.HoldWorkspaceLock(t, r.bus, r.s), r.schedule, r.remove)

	require.NoError(t, errs["schedule"])
	b := r.broadcast(t)
	var used *inuse.Error
	require.ErrorAs(t, errs["delete"], &used)
	assert.Equal(t, []inuse.User{{Kind: inuse.Broadcast, ID: b.ID, Name: b.Name}}, used.Users)
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
	out := outbound.New(bus, holds.New(func() bool { return true }, time.Now), anySender{}, tracking.New("test-secret", "http://local"))
	sends := broadcasts.New(bus, testhelper.Renderer(), out)
	integs := integrations.New(bus, nil, nil, nil, nil, "")

	_, err := sends.Update(ctx, s, fixtures.BroadcastDraftID, broadcasts.UpdateInput{IntegrationID: patch.Set[int64](fixtures.IntegrationAcmeGenericAID)})
	require.NoError(t, err)
	return integrationRace{
		sqlDB: sqlDB,
		bus:   bus,
		s:     s,
		sends: sends,
		remove: testhelper.Writer{Name: "delete", Run: func() error {
			return integs.Delete(ctx, s, fixtures.IntegrationAcmeGenericAID)
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

// Every removal that checks Live use and every write that creates it waits for the Workspace's
// lock (ADR 0099). What each one answers once it runs is its own module's concern; here it only has
// to wait.
func TestEveryCheckedRemovalAndEveryWriteThatCreatesLiveUseWaitsForTheWorkspaceLock(t *testing.T) {
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	s := entclient.New(sqlDB).Scoped(fixtures.AcmeID)
	bus := events.New(sqlDB, entclient.NewTx)
	bus.UseJobs(testhelper.AcceptingQueue{})
	out := outbound.New(bus, holds.New(func() bool { return true }, time.Now), anySender{}, tracking.New("test-secret", "http://local"))
	sends := broadcasts.New(bus, testhelper.Renderer(), out)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	integs := integrations.New(bus, cipher, nil, nil, nil, "")
	domains := sendingdomains.New(bus, cipher, nil, messaging.NewResolver(cipher, registry.New(cfg.SMTPAllowPrivateHosts, cfg.TelegramBotAPIURL, cfg.TwilioAPIURL)))
	segs := segments.New(bus)
	tagDeletion := tagdeletion.New(bus)
	ruleset := rules.New(cipher, bus)
	autos := automations.New(cipher, bus, testhelper.Renderer(), out)
	renamed := new("Renamed")

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"delete an Integration", func() error { return integs.Delete(ctx, s, fixtures.IntegrationAcmeGenericAID) }},
		{"delete a Sending domain", func() error { return domains.Delete(ctx, s, fixtures.SendingDomainUnverifiedID) }},
		{"delete a Tag", func() error { return tagDeletion.Delete(ctx, s, fixtures.TagUnusedID) }},
		{"archive a Segment", func() error { _, err := segs.Archive(ctx, s, fixtures.SegmentRetiredID); return err }},
		{"create a Segment", func() error { _, err := segs.Create(ctx, s, segments.CreateInput{Name: "New"}); return err }},
		{"update a Segment", func() error {
			_, err := segs.Update(ctx, s, fixtures.SegmentActiveID, segments.UpdateInput{Name: patch.Set("Renamed")})
			return err
		}},
		{"schedule a Broadcast", func() error {
			_, err := sends.Schedule(ctx, s, fixtures.BroadcastDraftID, time.Now().Add(time.Hour))
			return err
		}},
		{"send a Broadcast", func() error { _, err := sends.Send(ctx, s, fixtures.BroadcastScheduledID); return err }},
		{"archive a Broadcast", func() error { _, err := sends.Archive(ctx, s, fixtures.BroadcastFailedID); return err }},
		{"create a Rule", func() error { _, err := ruleset.Create(ctx, s, rules.CreateInput{Name: "New"}); return err }},
		{"update a Rule", func() error {
			_, err := ruleset.Update(ctx, s, fixtures.RuleInactiveID, rules.UpdateInput{Name: renamed})
			return err
		}},
		{"publish a Rule", func() error { _, err := ruleset.Publish(ctx, s, fixtures.RuleInactiveID); return err }},
		{"create an Automation", func() error { _, err := autos.Create(ctx, s, automations.CreateInput{Name: "New"}); return err }},
		{"update an Automation", func() error {
			_, err := autos.Update(ctx, s, fixtures.AutomationWinBackID, automations.UpdateInput{Name: renamed})
			return err
		}},
		{"publish an Automation", func() error { _, err := autos.Publish(ctx, s, fixtures.AutomationWinBackID); return err }},
		{"archive an Automation", func() error { _, err := autos.Archive(ctx, s, fixtures.AutomationJobsEditableID); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testhelper.QueuedBehind(t, sqlDB, testhelper.HoldWorkspaceLock(t, bus, s), testhelper.Writer{Name: tc.name, Run: tc.run})
		})
	}
}
