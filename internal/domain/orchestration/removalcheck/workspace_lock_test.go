package removalcheck_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/segmentsaving"
)

// Every write that creates a use a Removal check reads (Live use or Definition use), and the
// archive of a Segment, which a draft's schedule copies the Condition of, waits for the Workspace's
// lock (ADR 0099), so a removal that holds it never passes its check on a use made meanwhile. Each
// removal that runs the Removal check is raced against the writes it reads in its use case's own
// test. What each write answers once it runs is its own module's concern; here it only has to wait.
func TestEveryWriteThatCreatesAUseWaitsForTheWorkspaceLock(t *testing.T) {
	ctx := context.Background()
	env := testhelper.Setup(t, testhelper.Committed())
	s := env.DB.Scoped(fixtures.AcmeID)
	sends := testhelper.Module[*broadcasts.Module](env)
	segs := testhelper.Module[*segments.Module](env)
	save := testhelper.Module[*segmentsaving.UseCase](env)
	ruleset := testhelper.Module[*rules.Module](env)
	autos := testhelper.Module[*automations.Module](env)
	renamed := new("Renamed")

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"archive a Segment", func() error { _, err := segs.Archive(ctx, s, fixtures.SegmentRetiredID); return err }},
		{"create a Segment", func() error { _, err := save.Create(ctx, s, segments.CreateInput{Name: "New"}); return err }},
		{"update a Segment", func() error {
			_, err := save.Update(ctx, s, fixtures.SegmentActiveID, segments.UpdateInput{Revision: testhelper.SegmentRevisionIn(ctx, t, s, fixtures.SegmentActiveID), Name: patch.Set("Renamed")})
			return err
		}},
		{"schedule a Broadcast", func() error {
			_, err := sends.Schedule(ctx, s, fixtures.BroadcastDraftID, broadcasts.ScheduleInput{Revision: testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastDraftID), ScheduledAt: time.Now().Add(time.Hour)})
			return err
		}},
		{"send a Broadcast", func() error {
			_, err := sends.Send(ctx, s, fixtures.BroadcastScheduledID, testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastScheduledID))
			return err
		}},
		{"archive a Broadcast", func() error { _, err := sends.Archive(ctx, s, fixtures.BroadcastFailedID); return err }},
		{"create a Rule", func() error { _, err := ruleset.Create(ctx, s, rules.CreateInput{Name: "New"}); return err }},
		{"update a Rule", func() error {
			_, err := ruleset.Update(ctx, s, fixtures.RuleInactiveID, rules.UpdateInput{Revision: testhelper.RuleRevisionIn(ctx, t, s, fixtures.RuleInactiveID), Name: renamed})
			return err
		}},
		{"publish a Rule", func() error {
			_, err := ruleset.Publish(ctx, s, fixtures.RuleInactiveID, testhelper.RuleRevisionIn(ctx, t, s, fixtures.RuleInactiveID))
			return err
		}},
		{"create an Automation", func() error { _, err := autos.Create(ctx, s, automations.CreateInput{Name: "New"}); return err }},
		{"update an Automation", func() error {
			_, err := autos.Update(ctx, s, fixtures.AutomationWinBackID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationWinBackID), Name: renamed})
			return err
		}},
		{"publish an Automation", func() error {
			_, err := autos.Publish(ctx, s, fixtures.AutomationWinBackID, testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationWinBackID))
			return err
		}},
		{"archive an Automation", func() error { _, err := autos.Archive(ctx, s, fixtures.AutomationJobsEditableID); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env.QueueBehindWorkspaceLock(t, s, testhelper.Writer{Name: tc.name, Run: tc.run})
		})
	}
}
