package automations

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/drain"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// The entry checks (ADR 0127): before an Enrollment is made, under the Contact's lock it already
// holds, the entry is judged against the Contact's other Enrollments in the Automation, and a
// refused entry is recorded as a skipped entry with the reason of the first check that refused it.
// The checks run in a fixed order:
//
//  1. a Trigger filter miss (no entry at all, nothing recorded);
//  2. no Enrollment key property (`no_enrollment_key`);
//  3. a cycle (`cycle`);
//  4. an Enrollment active at the entry's moment (`active_enrollment`);
//  5. `once`, any earlier Enrollment (`once`);
//  6. the cooldown from the previous Enrollment's beginning (`cooldown`);
//  7. a keyed version's ceiling (`enrollment_ceiling`).
//
// entryChecks holds the ones built, in that order; a check that comes takes its place among them.
//
// A cycle is an entry whose cause already passed through the Automation: the Event, or the Event a
// Rule fired on, names it among the Automations its chain passed through (events.Envelope.Automations),
// so the Automation's own Tag, its own lifecycle Event or B enrolling back into A never enrolls the
// Contact again, however long the chain waited.
//
// The "other" Enrollments leave out the one the entry's own Event or Firing made: an Event handled
// again finds the Enrollment it began, which is no reason to skip it, and the insert's ON CONFLICT
// makes nothing (one Enrollment per Automation and Event, or Firing). An Enrollment made already
// ended, by an exit or goal Event recorded before it, counts like any other.

// entryCase is what an entry check judges: the Automation, the Automations its cause's chain passed
// through, the moment the entry would begin (its Event's occurred_at) and the Contact's other
// Enrollments in the Automation.
type entryCase struct {
	automationID int64
	chain        []int64
	at           time.Time
	others       []*ent.AutomationRun
}

// entryCheck is one entry check: the reason it skips the entry, or false when it lets it through.
type entryCheck func(c entryCase) (automationskippedentry.Reason, bool)

// entryChecks are the entry checks built, in the order they run.
var entryChecks = []entryCheck{
	cycle,
	activeEnrollment,
	onceEver,
}

// cycle skips an entry whose cause's chain already passed through the Automation.
func cycle(c entryCase) (automationskippedentry.Reason, bool) {
	if slices.Contains(c.chain, c.automationID) {
		return automationskippedentry.ReasonCycle, true
	}
	return "", false
}

// activeEnrollment skips an entry whose moment falls while another Enrollment of the Contact was
// active: begun at that moment or before and not ended by then. It judges the Event's moment, not
// the handling's, so a late or backdated Event does not start a second run of what it happened
// during.
func activeEnrollment(c entryCase) (automationskippedentry.Reason, bool) {
	for _, run := range c.others {
		begun := !run.BeganAt.After(c.at)
		ongoing := run.EndedAt == nil || c.at.Before(*run.EndedAt)
		if begun && ongoing {
			return automationskippedentry.ReasonActiveEnrollment, true
		}
	}
	return "", false
}

// onceEver skips an entry of a Contact that had any other Enrollment in the Automation: the `once`
// Entry policy, the only one built.
func onceEver(c entryCase) (automationskippedentry.Reason, bool) {
	if len(c.others) > 0 {
		return automationskippedentry.ReasonOnce, true
	}
	return "", false
}

// judgeEntry runs the entry checks on the Contact's entry into the Automation of the origin o and
// returns the reason of the first that refuses it, or false when every one lets it through.
func judgeEntry(ctx context.Context, s *ent.Scoped, automationID, contactID int64, o origin) (automationskippedentry.Reason, bool, error) {
	where := append([]predicate.AutomationRun{automationrun.AutomationID(automationID), automationrun.ContactID(contactID)}, notMadeBy(o)...)
	others, err := s.AutomationRun().Query().Where(where...).All(ctx)
	if err != nil {
		return "", false, err
	}
	c := entryCase{automationID: automationID, chain: o.Chain, at: o.OccurredAt, others: others}
	for _, check := range entryChecks {
		if reason, skipped := check(c); skipped {
			return reason, true, nil
		}
	}
	return "", false, nil
}

// notMadeBy leaves out the Enrollment the origin's Event or Firing made; an origin with neither
// made none.
func notMadeBy(o origin) []predicate.AutomationRun {
	switch {
	case o.SourceID != "":
		return []predicate.AutomationRun{automationrun.Or(automationrun.TriggerSourceIDIsNil(), automationrun.TriggerSourceIDNEQ(o.SourceID))}
	case o.FiringID != 0:
		return []predicate.AutomationRun{automationrun.Or(automationrun.FiringIDIsNil(), automationrun.FiringIDNEQ(o.FiringID))}
	}
	return nil
}

// recordSkip records the Contact's entry into the Automation of the origin o as skipped for the
// reason, once per Automation and Event (or Firing): an entry handled again records nothing new.
func recordSkip(ctx context.Context, s *ent.Scoped, automationID, contactID int64, o origin, reason automationskippedentry.Reason) error {
	create := s.AutomationSkippedEntry().Create().
		SetAutomationID(automationID).
		SetContactID(contactID).
		SetReason(reason).
		SetOccurredAt(o.OccurredAt)
	var err error
	switch {
	case o.SourceID != "":
		err = create.SetSourceID(o.SourceID).
			OnConflictColumns(automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID, automationskippedentry.FieldSourceID).
			DoNothing().Exec(ctx)
	case o.FiringID != 0:
		err = create.SetFiringID(o.FiringID).
			OnConflictColumns(automationskippedentry.FieldWorkspaceID, automationskippedentry.FieldAutomationID, automationskippedentry.FieldFiringID).
			DoNothing().Exec(ctx)
	default:
		err = create.Exec(ctx)
	}
	_, err = db.Inserted(err)
	return err
}

// The unique constraints of the entry identities (ADR 0106): no client can break them.
var _ = db.Declare(
	db.Fault("automation_runs_workspace_automation_trigger_source",
		"an Event belongs to one Contact, so a second Enrollment on it conflicts on the Automation and the Contact too, the insert's ON CONFLICT target"),
	db.Fault("automation_runs_workspace_automation_firing",
		"a Firing belongs to one Contact, so a second Enrollment on it conflicts on the Automation and the Contact too, the insert's ON CONFLICT target"),
	db.Fault("automation_skipped_entries_workspace_automation_source",
		"a skipped entry of an Event is written with ON CONFLICT DO NOTHING on this index"),
	db.Fault("automation_skipped_entries_workspace_automation_firing",
		"a skipped entry of a Firing is written with ON CONFLICT DO NOTHING on this index"),
)

// SkippedEntriesFilter names the Automation whose skipped entries are listed and, optionally, the
// one Contact they are of (0 for all).
type SkippedEntriesFilter struct {
	AutomationID int64
	ContactID    int64
}

// SkippedEntries returns one page of an Automation's skipped entries, newest first, optionally only
// those of one Contact. ErrNotFound when the Workspace has no such Automation.
func (m *Module) SkippedEntries(ctx context.Context, s *ent.Scoped, p pagination.Params, f SkippedEntriesFilter) (pagination.Page[*ent.AutomationSkippedEntry], error) {
	if _, err := s.Automation().Get(ctx, f.AutomationID); err != nil {
		return pagination.Page[*ent.AutomationSkippedEntry]{}, notFound(err)
	}
	q := s.AutomationSkippedEntry().Query().Where(automationskippedentry.AutomationID(f.AutomationID))
	if f.ContactID != 0 {
		q = q.Where(automationskippedentry.ContactID(f.ContactID))
	}
	return pagination.List(ctx, p, q.Order(ent.Desc(automationskippedentry.FieldOccurredAt), ent.Desc(automationskippedentry.FieldID)))
}

// PruneSkippedEntries deletes the skipped entries recorded more than retention before the Instance
// clock's now, in batches of batchSize, and returns how many it removed: they are analytical
// history, kept for the window of the analytical Events (ADR 0019). A retention of zero or less
// disables the prune. It spans every Workspace, so it takes the raw client.
func (m *StepExecution) PruneSkippedEntries(ctx context.Context, client *ent.Client, retention time.Duration, batchSize int) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := m.now().Add(-retention)
	total, err := drain.Batches(batchSize, func() (int, error) {
		ids, err := client.AutomationSkippedEntry.Query().
			Where(automationskippedentry.CreatedAtLT(cutoff)).
			Order(automationskippedentry.ByCreatedAt()).
			Limit(batchSize).
			IDs(ctx)
		if err != nil {
			return 0, fmt.Errorf("select expired skipped entries batch: %w", err)
		}
		if len(ids) == 0 {
			return 0, nil
		}
		n, err := client.AutomationSkippedEntry.Delete().Where(automationskippedentry.IDIn(ids...)).Exec(ctx)
		if err != nil {
			return 0, fmt.Errorf("delete expired skipped entries batch: %w", err)
		}
		return n, nil
	})
	return int64(total), err
}
