package workspacedeletion

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	"github.com/getsphericon/sphericon/ent/erasure"
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
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// eraseBatch bounds the rows one transaction removes, so a large Workspace never holds a
// long lock and a crash resumes where it stopped.
const eraseBatch = 500

// maxErasePasses bounds the passes eraseWorkspace makes over the steps. Deleting a row can
// write another (an ent hook, the bus), so one pass can leave rows behind; each pass removes
// what the one before it caused, so it settles in a few.
const maxErasePasses = 10

// EraseDue erases every Workspace whose deletion window has passed at now and returns how
// many it finished. Each Workspace's rows go in batches, each batch in a transaction of its
// own through the event bus, so the ent hooks run (the tracking host change log records the
// released hosts, the workspace feed advances) and a crash resumes on the rows that are
// left. The last transaction writes the tombstone (Erase). A Workspace that fails does not
// stop the others; the errors are joined. The scope of each step is built here, after the
// Workspace's row is listed; it has no actor, so the deletes are not audited.
func (m *Module) EraseDue(ctx context.Context, now time.Time) (int, error) {
	ids, err := m.Due(ctx, now)
	if err != nil {
		return 0, err
	}
	var errs []error
	erased := 0
	for _, id := range ids {
		if err := m.eraseWorkspace(ctx, id, now); err != nil {
			// A restore that won the race moved the Workspace out of pending deletion: the
			// stale move is finished, not an error (ADR 0046).
			if !errors.Is(err, lifecycle.ErrInvalidTransition) {
				errs = append(errs, fmt.Errorf("erase workspace %d: %w", id, err))
			}
			continue
		}
		erased++
	}
	return erased, errors.Join(errs...)
}

func (m *Module) eraseWorkspace(ctx context.Context, id int64, now time.Time) error {
	// Rerun every step until a whole pass finds nothing to remove: a row written while
	// another kind was being deleted is then removed too, before the tombstone is made.
	settled := false
	for pass := 0; pass < maxErasePasses && !settled; pass++ {
		removed, err := m.erasePass(ctx, id)
		if err != nil {
			return err
		}
		settled = removed == 0
	}
	if !settled {
		return fmt.Errorf("rows are still being written after %d passes", maxErasePasses)
	}
	return m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		if _, err := tx.OAuthCode.Delete().Where(oauthcode.WorkspaceID(id)).Exec(ctx); err != nil {
			return err
		}
		return m.Erase(ctx, tx, id, now)
	})
}

// erasePass runs every step once, each in batches, and returns how many rows it removed.
func (m *Module) erasePass(ctx context.Context, id int64) (int, error) {
	total := 0
	for _, step := range eraseSteps {
		for {
			removed := 0
			err := m.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
				s := tx.Scoped(id)
				rows, err := step.ids(ctx, s, m.batch)
				if err != nil || len(rows) == 0 {
					return err
				}
				removed = len(rows)
				return step.remove(ctx, s, rows)
			})
			if err != nil {
				return total, err
			}
			total += removed
			if removed < m.batch {
				break
			}
		}
	}
	return total, nil
}

// eraseStep removes one kind of Workspace-owned row, a batch at a time: ids names up
// to limit of the Workspace's rows, remove deletes those by id.
type eraseStep struct {
	ids    func(ctx context.Context, s *ent.Scoped, limit int) ([]int64, error)
	remove func(ctx context.Context, s *ent.Scoped, ids []int64) error
}

// eraseSteps is every Workspace-owned entity in dependency order (children before the
// rows they reference): the foreign keys are not cascading. A test checks it covers every
// entity of ent.ScopedEntities.
var eraseSteps = []eraseStep{
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
			return s.Erasure().Query().Limit(n).IDs(ctx)
		},
		func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := s.Erasure().Delete().Where(erasure.IDIn(ids...)).Exec(ctx)
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
