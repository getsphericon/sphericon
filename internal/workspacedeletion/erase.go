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
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/brandkit"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/ent/erasure"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/heldorderevent"
	"github.com/getsphericon/sphericon/ent/hold"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/joindomain"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/ordermovement"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/ent/ruleversion"
	"github.com/getsphericon/sphericon/ent/ruleversiontrigger"
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
// left. The last transaction writes the tombstone (tombstone). A Workspace that fails does not
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

// eraseWorkspace removes the Workspace's rows, then writes the tombstone. The tombstone's
// compare-and-swap cannot lose after the steps in practice: Due selects only rows past the
// window and Restore is refused once the window has passed, so nothing moves a listed row
// back to active. If it did, the stale move is finished as ErrInvalidTransition (EraseDue).
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
		return tombstone(ctx, tx, id, now)
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

// rowQuery is the scoped Query builder of one entity: the part an erase step needs.
type rowQuery[Q any] interface {
	Limit(limit int) Q
	IDs(ctx context.Context) ([]int64, error)
}

// rowDelete is the scoped Delete builder of one entity, filtered by its id predicate P.
type rowDelete[D, P any] interface {
	Where(predicates ...P) D
	Exec(ctx context.Context) (int, error)
}

// scopedTable is one entity's scoped wrapper (`s.AuditEntry()`).
type scopedTable[Q, D any] interface {
	Query() Q
	Delete() D
}

// stepOf is the erase step of one entity: table is its scoped wrapper (a method expression
// such as (*ent.Scoped).Contact) and idIn its generated id predicate (contact.IDIn).
func stepOf[W scopedTable[Q, D], Q rowQuery[Q], D rowDelete[D, P], P any](
	table func(*ent.Scoped) W, idIn func(...int64) P,
) eraseStep {
	return eraseStepOf(
		func(s *ent.Scoped) Q { return table(s).Query() },
		func(s *ent.Scoped) D { return table(s).Delete() },
		idIn,
	)
}

// discardableTable is the scoped wrapper of an entity with the DiscardMixin (ADR 0049).
type discardableTable[Q, D any] interface {
	QueryWithDiscarded() Q
	Purge() D
}

// purgeStepOf is the erase step of a discardable entity: Workspace erasure sees discarded
// rows too and removes them with Purge.
func purgeStepOf[W discardableTable[Q, D], Q rowQuery[Q], D rowDelete[D, P], P any](
	table func(*ent.Scoped) W, idIn func(...int64) P,
) eraseStep {
	return eraseStepOf(
		func(s *ent.Scoped) Q { return table(s).QueryWithDiscarded() },
		func(s *ent.Scoped) D { return table(s).Purge() },
		idIn,
	)
}

func eraseStepOf[Q rowQuery[Q], D rowDelete[D, P], P any](
	query func(*ent.Scoped) Q, del func(*ent.Scoped) D, idIn func(...int64) P,
) eraseStep {
	return eraseStep{
		ids: func(ctx context.Context, s *ent.Scoped, limit int) ([]int64, error) {
			return query(s).Limit(limit).IDs(ctx)
		},
		remove: func(ctx context.Context, s *ent.Scoped, ids []int64) error {
			_, err := del(s).Where(idIn(ids...)).Exec(ctx)
			return err
		},
	}
}

// eraseSteps is every Workspace-owned entity in dependency order (children before the
// rows they reference): the foreign keys are not cascading. A test checks it covers every
// entity of ent.ScopedEntities.
var eraseSteps = []eraseStep{
	stepOf((*ent.Scoped).AuditEntry, auditentry.IDIn),
	stepOf((*ent.Scoped).Erasure, erasure.IDIn),
	stepOf((*ent.Scoped).Event, event.IDIn),
	stepOf((*ent.Scoped).HeldOrderEvent, heldorderevent.IDIn),
	stepOf((*ent.Scoped).OrderMovement, ordermovement.IDIn),
	stepOf((*ent.Scoped).Order, order.IDIn),
	stepOf((*ent.Scoped).CapturedMessage, capturedmessage.IDIn),
	stepOf((*ent.Scoped).OutboundMessage, outboundmessage.IDIn),
	stepOf((*ent.Scoped).BroadcastRecipient, broadcastrecipient.IDIn),
	stepOf((*ent.Scoped).RuleFiring, rulefiring.IDIn),
	stepOf((*ent.Scoped).RuleVersionTrigger, ruleversiontrigger.IDIn),
	stepOf((*ent.Scoped).RuleVersion, ruleversion.IDIn),
	purgeStepOf((*ent.Scoped).Rule, rule.IDIn),
	stepOf((*ent.Scoped).AutomationRun, automationrun.IDIn),
	stepOf((*ent.Scoped).AutomationVersion, automationversion.IDIn),
	stepOf((*ent.Scoped).Visitor, visitor.IDIn),
	stepOf((*ent.Scoped).Confirmation, confirmation.IDIn),
	stepOf((*ent.Scoped).Unsubscribe, unsubscribe.IDIn),
	stepOf((*ent.Scoped).Suppression, suppression.IDIn),
	stepOf((*ent.Scoped).Usage, usage.IDIn),
	stepOf((*ent.Scoped).Hold, hold.IDIn),
	stepOf((*ent.Scoped).SendLimiter, sendlimiter.IDIn),
	stepOf((*ent.Scoped).WebhookEndpoint, webhookendpoint.IDIn),
	stepOf((*ent.Scoped).Invitation, invitation.IDIn),
	stepOf((*ent.Scoped).ApiToken, apitoken.IDIn),
	purgeStepOf((*ent.Scoped).Broadcast, broadcast.IDIn),
	purgeStepOf((*ent.Scoped).Automation, automation.IDIn),
	purgeStepOf((*ent.Scoped).Segment, segment.IDIn),
	stepOf((*ent.Scoped).EmailTemplate, emailtemplate.IDIn),
	stepOf((*ent.Scoped).BrandKit, brandkit.IDIn),
	stepOf((*ent.Scoped).Contact, contact.IDIn),
	stepOf((*ent.Scoped).Tag, tag.IDIn),
	stepOf((*ent.Scoped).CustomField, customfield.IDIn),
	// The tracking host change log hook records each released host here.
	purgeStepOf((*ent.Scoped).SendingDomain, sendingdomain.IDIn),
	purgeStepOf((*ent.Scoped).Integration, integration.IDIn),
	stepOf((*ent.Scoped).JoinDomain, joindomain.IDIn),
	// Owner Memberships last: the owner hook advances the feed as they go.
	stepOf((*ent.Scoped).Membership, membership.IDIn),
}
