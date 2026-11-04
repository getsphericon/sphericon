package broadcasts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
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
func TestDeletingAnIntegrationAndSchedulingABroadcastThroughItTakeTurns(t *testing.T) {
	for _, tc := range []struct {
		name  string
		queue func(remove, schedule testhelper.Writer) []testhelper.Writer
		check func(t *testing.T, errs map[string]error, b *ent.Broadcast)
	}{
		{
			name: "the delete queues first and the schedule is refused",
			queue: func(remove, schedule testhelper.Writer) []testhelper.Writer {
				return []testhelper.Writer{remove, schedule}
			},
			check: func(t *testing.T, errs map[string]error, b *ent.Broadcast) {
				t.Helper()
				require.NoError(t, errs["delete"])
				require.ErrorIs(t, errs["schedule"], outbound.ErrRouteHeld)
				assert.Equal(t, broadcast.StatusDraft, b.Status)
			},
		},
		{
			name: "the schedule queues first and the delete is refused",
			queue: func(remove, schedule testhelper.Writer) []testhelper.Writer {
				return []testhelper.Writer{schedule, remove}
			},
			check: func(t *testing.T, errs map[string]error, b *ent.Broadcast) {
				t.Helper()
				require.NoError(t, errs["schedule"])
				var used *inuse.Error
				require.ErrorAs(t, errs["delete"], &used)
				assert.Equal(t, []inuse.User{{Kind: inuse.Broadcast, ID: b.ID, Name: b.Name}}, used.Users)
				assert.Equal(t, broadcast.StatusScheduled, b.Status)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			sqlDB := testhelper.RealDB(t)
			sqlDB.SetMaxOpenConns(8)
			s := db.NewEntClient(sqlDB).Scoped(fixtures.AcmeID)
			bus := events.New(sqlDB)
			bus.UseJobs(testhelper.AcceptingQueue{})
			out := outbound.New(bus, holds.New(func() bool { return true }, time.Now), anySender{}, tracking.New("test-secret", "http://local"))
			sends := broadcasts.New(bus, testhelper.Renderer(), out)
			integs := integrations.New(bus, nil, nil, nil, nil, "")

			_, err := sends.Update(ctx, s, fixtures.BroadcastDraftID, broadcasts.UpdateInput{IntegrationID: patch.Set[int64](fixtures.IntegrationAcmeGenericAID)})
			require.NoError(t, err)
			remove := testhelper.Writer{Name: "delete", Run: func() error {
				return integs.Delete(ctx, s, fixtures.IntegrationAcmeGenericAID)
			}}
			schedule := testhelper.Writer{Name: "schedule", Run: func() error {
				_, err := sends.Schedule(ctx, s, fixtures.BroadcastDraftID, time.Now().Add(time.Hour))
				return err
			}}

			errs := testhelper.QueuedBehind(t, sqlDB, testhelper.HoldWorkspaceLock(t, bus, s), tc.queue(remove, schedule)...)

			b, err := sends.Get(ctx, s, fixtures.BroadcastDraftID)
			require.NoError(t, err)
			tc.check(t, errs, b)
		})
	}
}

// Every removal that checks Live use and every write that creates it waits for the Workspace's
// lock (ADR 0099). What each one answers once it runs is its own module's concern; here it only has
// to wait.
func TestEveryCheckedRemovalAndEveryWriteThatCreatesLiveUseWaitsForTheWorkspaceLock(t *testing.T) {
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	s := db.NewEntClient(sqlDB).Scoped(fixtures.AcmeID)
	bus := events.New(sqlDB)
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
	catalogue := tags.New(bus)
	ruleset := rules.New(cipher, bus)
	autos := automations.New(cipher, bus, testhelper.Renderer(), out)
	renamed := new("Renamed")

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"delete an Integration", func() error { return integs.Delete(ctx, s, fixtures.IntegrationAcmeGenericAID) }},
		{"delete a Sending domain", func() error { return domains.Delete(ctx, s, fixtures.SendingDomainUnverifiedID) }},
		{"delete a Tag", func() error { return catalogue.Delete(ctx, s, fixtures.TagUnusedID) }},
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
