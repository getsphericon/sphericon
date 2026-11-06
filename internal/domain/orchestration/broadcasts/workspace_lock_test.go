package broadcasts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/domain/sending/tracking"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/integrationdeletion"
	"github.com/getsphericon/sphericon/internal/usecase/sendingdomaindeletion"
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
	out := outboundmessages.New(bus, holds.New(func() bool { return true }, time.Now), anySender{}, tracking.New("test-secret", "http://local"))
	sends := broadcasts.New(bus, testhelper.Renderer(), out)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	integrationDeletion := integrationdeletion.New(bus, integrations.New(bus, cipher, nil, nil, nil, ""))
	domainDeletion := sendingdomaindeletion.New(bus, sendingdomains.New(bus, cipher), messaging.NewResolver(cipher, registry.New(cfg.SMTPAllowPrivateHosts, cfg.TelegramBotAPIURL, cfg.TwilioAPIURL)))
	segs := segments.New(bus)
	tagDeletion := tagdeletion.New(bus)
	ruleset := rules.New(cipher, bus)
	autos := automations.New(cipher, bus, testhelper.Renderer(), out)
	renamed := new("Renamed")

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"delete an Integration", func() error { return integrationDeletion.Delete(ctx, s, fixtures.IntegrationAcmeGenericAID) }},
		{"delete a Sending domain", func() error { return domainDeletion.Delete(ctx, s, fixtures.SendingDomainUnverifiedID) }},
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
