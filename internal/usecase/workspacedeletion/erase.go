package workspacedeletion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppression"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/contacts/visitors"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkit"
	"github.com/getsphericon/sphericon/internal/domain/content/templates"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/domain/identity/joindomains"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/oauthclients"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/sending/capturedmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outbound"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackingdomains"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/webhooks"
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
// left. The last transaction removes the OAuth codes that name the Workspace and writes the
// tombstone (workspaces.TombstoneIn). A Workspace that fails does not stop the others; the
// errors are joined. The scope of each step is built here, after the
// Workspace's row is listed; it has no actor, so the deletes are not audited.
func (m *UseCase) EraseDue(ctx context.Context, now time.Time) (int, error) {
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
func (m *UseCase) eraseWorkspace(ctx context.Context, id int64, now time.Time) error {
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
		if err := oauthclients.DeleteCodesIn(ctx, tx, id); err != nil {
			return err
		}
		return workspaces.TombstoneIn(ctx, tx, id, now)
	})
}

// erasePass runs every step once, each in batches, and returns how many rows it removed.
func (m *UseCase) erasePass(ctx context.Context, id int64) (int, error) {
	total := 0
	for _, step := range m.steps {
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
	remove RowEraser
}

// rowQuery is the scoped Query builder of one entity: the part an erase step needs.
type rowQuery[Q any] interface {
	Limit(limit int) Q
	IDs(ctx context.Context) ([]int64, error)
}

// RowEraser removes the scoped Workspace's rows of one entity with the ids, inside the
// transaction s is bound to: the removal of the module that owns the table (ADR 0102: only the
// owner writes a table).
type RowEraser func(ctx context.Context, s *ent.Scoped, ids []int64) error

// EditionTables are the removals of the Workspace-owned tables the Enterprise Edition owns: the
// Audit entries (ee/audit), the Erasures rows (ee/export) and the Usage rows (ee/usage). No core
// package imports the Edition, so the wiring hands them in; they run at their place in the order
// like every other step, whatever the license.
type EditionTables struct {
	AuditEntries, Erasures, Usage RowEraser
}

// stepOf is the erase step of one entity: table is its scoped wrapper (a method expression such
// as (*ent.Scoped).Contact), whose Query lists the rows, and remove its owner's removal.
func stepOf[W interface{ Query() Q }, Q rowQuery[Q]](table func(*ent.Scoped) W, remove RowEraser) eraseStep {
	return eraseStepOf(func(s *ent.Scoped) Q { return table(s).Query() }, remove)
}

// keptStepOf is the erase step of a discardable or an archivable entity (ADR 0100): every names
// the query that sees its discarded or archived rows too ((*ent.RuleScoped).QueryWithDiscarded,
// QueryWithArchived), and its owner's removal purges them all.
func keptStepOf[W any, Q rowQuery[Q]](table func(*ent.Scoped) W, every func(W) Q, remove RowEraser) eraseStep {
	return eraseStepOf(func(s *ent.Scoped) Q { return every(table(s)) }, remove)
}

func eraseStepOf[Q rowQuery[Q]](query func(*ent.Scoped) Q, remove RowEraser) eraseStep {
	return eraseStep{
		ids: func(ctx context.Context, s *ent.Scoped, limit int) ([]int64, error) {
			return query(s).Limit(limit).IDs(ctx)
		},
		remove: remove,
	}
}

// eraseSteps is every Workspace-owned entity in dependency order (children before the
// rows they reference): the foreign keys are not cascading. A test checks it covers every
// entity of ent.ScopedEntities. Each step removes through the owner of its table; the
// Edition's own tables are ed's.
func eraseSteps(ed EditionTables) []eraseStep {
	return []eraseStep{
		stepOf((*ent.Scoped).AuditEntry, ed.AuditEntries),
		stepOf((*ent.Scoped).Erasure, ed.Erasures),
		stepOf((*ent.Scoped).Event, events.EraseEventsIn),
		stepOf((*ent.Scoped).HeldOrderEvent, orders.EraseHeldOrderEventsIn),
		stepOf((*ent.Scoped).OrderMovement, orders.EraseOrderMovementsIn),
		stepOf((*ent.Scoped).Order, orders.EraseOrdersIn),
		stepOf((*ent.Scoped).CapturedMessage, capturedmessages.EraseCapturedMessagesIn),
		stepOf((*ent.Scoped).TrackedLink, outbound.EraseTrackedLinksIn),
		stepOf((*ent.Scoped).OutboundMessage, outbound.EraseOutboundMessagesIn),
		stepOf((*ent.Scoped).BroadcastRecipient, broadcasts.EraseBroadcastRecipientsIn),
		stepOf((*ent.Scoped).RuleFiring, rules.EraseRuleFiringsIn),
		stepOf((*ent.Scoped).RuleVersionTrigger, rules.EraseRuleVersionTriggersIn),
		stepOf((*ent.Scoped).RuleVersion, rules.EraseRuleVersionsIn),
		keptStepOf((*ent.Scoped).Rule, (*ent.RuleScoped).QueryWithDiscarded, rules.EraseRulesIn),
		stepOf((*ent.Scoped).AutomationRun, automations.EraseAutomationRunsIn),
		stepOf((*ent.Scoped).AutomationVersion, automations.EraseAutomationVersionsIn),
		stepOf((*ent.Scoped).Visitor, visitors.EraseVisitorsIn),
		stepOf((*ent.Scoped).Confirmation, confirmations.EraseConfirmationsIn),
		stepOf((*ent.Scoped).Unsubscribe, unsubscribes.EraseUnsubscribesIn),
		stepOf((*ent.Scoped).Suppression, suppression.EraseSuppressionsIn),
		stepOf((*ent.Scoped).Usage, ed.Usage),
		stepOf((*ent.Scoped).Hold, holds.EraseHoldsIn),
		stepOf((*ent.Scoped).ProviderLimiter, providerlimit.EraseProviderLimitersIn),
		stepOf((*ent.Scoped).SendCapLimiter, providerlimit.EraseSendCapLimitersIn),
		stepOf((*ent.Scoped).WebhookEndpoint, webhooks.EraseWebhookEndpointsIn),
		stepOf((*ent.Scoped).Invitation, invitations.EraseInvitationsIn),
		stepOf((*ent.Scoped).ApiToken, apitokens.EraseAPITokensIn),
		keptStepOf((*ent.Scoped).Broadcast, (*ent.BroadcastScoped).QueryWithArchived, broadcasts.EraseBroadcastsIn),
		keptStepOf((*ent.Scoped).Automation, (*ent.AutomationScoped).QueryWithArchived, automations.EraseAutomationsIn),
		keptStepOf((*ent.Scoped).Segment, (*ent.SegmentScoped).QueryWithArchived, segments.EraseSegmentsIn),
		stepOf((*ent.Scoped).EmailTemplate, templates.EraseEmailTemplatesIn),
		stepOf((*ent.Scoped).BrandKit, brandkit.EraseBrandKitsIn),
		stepOf((*ent.Scoped).Contact, contacts.EraseContactsIn),
		stepOf((*ent.Scoped).Tag, tags.EraseTagsIn),
		stepOf((*ent.Scoped).CustomField, contacts.EraseCustomFieldsIn),
		// The tracking host change log hook records each released host here.
		stepOf((*ent.Scoped).TrackingDomain, trackingdomains.EraseTrackingDomainsIn),
		keptStepOf((*ent.Scoped).SendingDomain, (*ent.SendingDomainScoped).QueryWithDiscarded, sendingdomains.EraseSendingDomainsIn),
		keptStepOf((*ent.Scoped).Integration, (*ent.IntegrationScoped).QueryWithDiscarded, integrations.EraseIntegrationsIn),
		stepOf((*ent.Scoped).JoinDomain, joindomains.EraseJoinDomainsIn),
		// Owner Memberships last: the owner hook advances the feed as they go.
		stepOf((*ent.Scoped).Membership, memberships.EraseMembershipsIn),
	}
}
