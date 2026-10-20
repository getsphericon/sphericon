package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/brandkit"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/hold"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/ent/sendlimiter"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/usage"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/events"
)

// workspaceErasureInterval is how often the erasure job looks for Workspaces whose
// deletion window has passed; the window is 30 days, so hourly is ample.
const workspaceErasureInterval = time.Hour

// workspaceErasureBatch bounds the rows one transaction removes, so a large Workspace
// never holds a long lock and a crash resumes where it stopped.
const workspaceErasureBatch = 500

// EraseWorkspacesArgs is the periodic tick that erases Workspaces whose deletion
// window has passed (ADR 0043).
type EraseWorkspacesArgs struct{}

func (EraseWorkspacesArgs) Kind() string { return "workspace_erasure" }

// EraseWorkspacesWorker erases what a deleted Workspace owned and leaves its row as a
// tombstone. It is an Instance-wide job entry point: it lists Workspaces with the raw
// client, then scopes to each one.
type EraseWorkspacesWorker struct {
	river.WorkerDefaults[EraseWorkspacesArgs]
	ent   *ent.Client
	bus   *events.Bus
	now   func() time.Time
	batch int
}

func (w *EraseWorkspacesWorker) Work(ctx context.Context, _ *river.Job[EraseWorkspacesArgs]) error {
	n, err := EraseWorkspaces(ctx, w.ent, w.bus, w.now(), w.batch)
	if n > 0 {
		slog.InfoContext(ctx, "workspaces erased", "count", n)
	}
	return err
}

// erasureStep removes one kind of Workspace-owned row, a batch at a time: ids names up
// to limit of the Workspace's rows, remove deletes those by id.
type erasureStep struct {
	ids    func(ctx context.Context, s *ent.Scoped, limit int) ([]int64, error)
	remove func(ctx context.Context, s *ent.Scoped, ids []int64) error
}

// erasureSteps is every Workspace-owned entity in dependency order (children before the
// rows they reference): the foreign keys are not cascading. A test checks it covers every
// entity of ent.ScopedEntities.
var erasureSteps = []erasureStep{
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.AuditEntry().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.AuditEntry().Delete().Where(auditentry.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Event().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Event().Delete().Where(event.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.OutboundMessage().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.OutboundMessage().Delete().Where(outboundmessage.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.BroadcastRecipient().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.BroadcastRecipient().Delete().Where(broadcastrecipient.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.AutomationRun().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.AutomationRun().Delete().Where(automationrun.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Visitor().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Visitor().Delete().Where(visitor.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Confirmation().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Confirmation().Delete().Where(confirmation.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Unsubscribe().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Unsubscribe().Delete().Where(unsubscribe.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Suppression().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Suppression().Delete().Where(suppression.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Usage().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Usage().Delete().Where(usage.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Hold().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Hold().Delete().Where(hold.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.SendLimiter().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.SendLimiter().Delete().Where(sendlimiter.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.WebhookEndpoint().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.WebhookEndpoint().Delete().Where(webhookendpoint.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Invitation().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Invitation().Delete().Where(invitation.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.ApiToken().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.ApiToken().Delete().Where(apitoken.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Broadcast().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Broadcast().Delete().Where(broadcast.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Automation().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Automation().Delete().Where(automation.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Segment().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Segment().Delete().Where(segment.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.EmailTemplate().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.EmailTemplate().Delete().Where(emailtemplate.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.BrandKit().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.BrandKit().Delete().Where(brandkit.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Contact().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Contact().Delete().Where(contact.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Tag().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Tag().Delete().Where(tag.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.CustomField().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.CustomField().Delete().Where(customfield.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		// The tracking host change log hook records each released host here.
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.SendingDomain().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.SendingDomain().Delete().Where(sendingdomain.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Integration().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Integration().Delete().Where(integration.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
	{
		// Owner Memberships last: the owner hook advances the feed as they go.
		func(ctx context.Context, s *ent.Scoped, n int) ([]int64, error) {
			return s.Membership().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Membership().Delete().Where(membership.IDIn(ids...)).Exec(ctx)
			return err
		},
	},
}

// EraseWorkspaces erases every Workspace whose deletion window has passed at now and
// returns how many it finished. Each Workspace's rows go in batches of at most batch,
// each batch in a transaction of its own through the event bus, so the ent hooks run
// (the tracking host change log records the released hosts, the workspace feed
// advances) and a crash resumes on the rows that are left. The last transaction blanks
// the name, rotates the collect key and sets erased_at: the row stays as a tombstone
// (ADR 0043). A Workspace that fails does not stop the others; the errors are joined.
func EraseWorkspaces(ctx context.Context, client *ent.Client, bus *events.Bus, now time.Time, batch int) (int, error) {
	ids, err := client.Workspace.Query().
		Where(workspace.DeletedAtLTE(now.Add(-accounts.DeletionWindow)), workspace.ErasedAtIsNil()).
		Order(workspace.ByID()).
		IDs(ctx)
	if err != nil {
		return 0, err
	}
	var errs []error
	erased := 0
	for _, id := range ids {
		if err := eraseWorkspace(ctx, client, bus, id, now, batch); err != nil {
			errs = append(errs, fmt.Errorf("erase workspace %d: %w", id, err))
			continue
		}
		erased++
	}
	return erased, errors.Join(errs...)
}

func eraseWorkspace(ctx context.Context, client *ent.Client, bus *events.Bus, id int64, now time.Time, batch int) error {
	// The raw client builds the scope after the row that names the Workspace is loaded; it
	// has no actor, so the deletes below are not audited.
	for _, step := range erasureSteps {
		for {
			removed := 0
			err := bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
				s := tx.Scoped(id)
				rows, err := step.ids(ctx, s, batch)
				if err != nil || len(rows) == 0 {
					return err
				}
				removed = len(rows)
				return step.remove(ctx, s, rows)
			})
			if err != nil {
				return err
			}
			if removed < batch {
				break
			}
		}
	}
	key, err := credentials.GenerateCollectKey()
	if err != nil {
		return err
	}
	return bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		if _, err := tx.OAuthCode.Delete().Where(oauthcode.WorkspaceID(id)).Exec(ctx); err != nil {
			return err
		}
		return tx.Workspace.UpdateOneID(id).
			SetName("").
			SetPostalAddress("").
			SetCollectKey(key).
			ClearRetentionDays().
			SetErasedAt(now).
			Exec(ctx)
	})
}
