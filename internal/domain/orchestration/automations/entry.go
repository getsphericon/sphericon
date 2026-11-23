package automations

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/automationversion"
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
// The first is enrollOnTrigger's admission, before the others are judged (matchesEvent);
// entryChecks holds the others built, in that order; a check that comes takes its place among them.
// `once` and the cooldown are the Entry policy of the version the Enrollment would begin on: `once`
// refuses any earlier Enrollment, `again` refuses one within the cooldown of another's beginning.
//
// A cycle is an entry whose cause already passed through the Automation: the Event, or the Event a
// Rule fired on, names it among the Automations its chain passed through (events.Envelope.Automations),
// so the Automation's own Tag, its own lifecycle Event or B enrolling back into A never enrolls the
// Contact again, however long the chain waited.
//
// The "other" Enrollments leave out the one the entry's own Event or Firing made: an Event handled
// again finds the Enrollment it began, which is no reason to skip it, and the insert's ON CONFLICT
// makes nothing (one Enrollment per Automation and Event, or Firing). An Enrollment made already
// ended, by an exit or goal Event recorded before it, counts like any other, for the checks and for
// the ordinal.

// entryCase is what an entry check judges: the Automation, the Entry policy and cooldown of the
// version the Enrollment would begin on, the Automations its cause's chain passed through, the
// moment the entry would begin (its Event's occurred_at) and the Contact's other Enrollments in the
// Automation.
type entryCase struct {
	automationID int64
	policy       automationversion.EntryPolicy
	cooldown     time.Duration
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
	cooldown,
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
// during. An Enrollment still active skips it too, whenever it began: a Contact has at most one
// active Enrollment in an Automation, so an Event older than that Enrollment does not start another
// beside it.
func activeEnrollment(c entryCase) (automationskippedentry.Reason, bool) {
	for _, run := range c.others {
		begun := !run.BeganAt.After(c.at)
		ongoing := run.EndedAt == nil || c.at.Before(*run.EndedAt)
		if run.Status == automationrun.StatusActive || begun && ongoing {
			return automationskippedentry.ReasonActiveEnrollment, true
		}
	}
	return "", false
}

// onceEver skips, under the `once` Entry policy, an entry of a Contact that had any other
// Enrollment in the Automation.
func onceEver(c entryCase) (automationskippedentry.Reason, bool) {
	if c.policy == automationversion.EntryPolicyOnce && len(c.others) > 0 {
		return automationskippedentry.ReasonOnce, true
	}
	return "", false
}

// cooldown skips, under the `again` Entry policy, an entry whose moment is less than the cooldown
// from another Enrollment's beginning: it caps how often a Contact enrolls, whatever the length of
// the path. The distance counts either way, so a backdated Event does not slip in just before an
// Enrollment begun after it either.
func cooldown(c entryCase) (automationskippedentry.Reason, bool) {
	if c.policy != automationversion.EntryPolicyAgain {
		return "", false
	}
	for _, run := range c.others {
		if c.at.Sub(run.BeganAt).Abs() < c.cooldown {
			return automationskippedentry.ReasonCooldown, true
		}
	}
	return "", false
}

// entry is what the entry checks made of an entry: the reason of the first that refused it, when
// skipped, and the ordinal the Enrollment it makes takes otherwise.
type entry struct {
	reason  automationskippedentry.Reason
	skipped bool
	ordinal int
}

// judgeEntry runs the entry checks on the Contact's entry, for the origin o, into the Automation
// whose live version v the Enrollment would begin on. Every Enrollment of the Contact in the
// Automation counts for the ordinal, the one o's own Event or Firing made included: the insert of
// an entry handled again makes nothing.
func judgeEntry(ctx context.Context, s *ent.Scoped, v *ent.AutomationVersion, contactID int64, o origin) (entry, error) {
	runs, err := s.AutomationRun().Query().
		Where(automationrun.AutomationID(v.AutomationID), automationrun.ContactID(contactID)).
		All(ctx)
	if err != nil {
		return entry{}, err
	}
	c := entryCase{
		automationID: v.AutomationID,
		policy:       v.EntryPolicy,
		cooldown:     time.Duration(v.CooldownSeconds) * time.Second,
		chain:        o.Chain,
		at:           o.OccurredAt,
		others:       lo.Reject(runs, func(run *ent.AutomationRun, _ int) bool { return madeBy(run, o) }),
	}
	next := entry{ordinal: 1}
	if len(runs) > 0 {
		next.ordinal = lo.MaxBy(runs, func(a, b *ent.AutomationRun) bool { return a.Ordinal > b.Ordinal }).Ordinal + 1
	}
	for _, check := range entryChecks {
		if reason, skipped := check(c); skipped {
			return entry{reason: reason, skipped: true}, nil
		}
	}
	return next, nil
}

// madeBy is whether the origin's Event or Firing made the Enrollment; an origin with neither made
// none.
func madeBy(run *ent.AutomationRun, o origin) bool {
	switch {
	case o.SourceID != "":
		return lo.FromPtr(run.TriggerSourceID) == o.SourceID
	case o.FiringID != 0:
		return lo.FromPtr(run.FiringID) == o.FiringID
	}
	return false
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
		"an Enrollment on an Event is written with ON CONFLICT DO NOTHING on this index: an Event handled again makes nothing"),
	db.Fault("automation_runs_workspace_automation_firing",
		"an Enrollment on a Firing is written with ON CONFLICT DO NOTHING on this index: a Firing run again makes nothing"),
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
