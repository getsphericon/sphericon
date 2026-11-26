package automations

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationoccurrencecursor"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/contactdate"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/optional"
)

// The date scheduler (ADR 0126): the occurrences of an Automation's date Trigger are decided by
// its cursor, which the Publish of a date Trigger resets (resetCursor) and each tick moves
// (DecideOccurrences).

// The unique constraint of the cursor (ADR 0106): no client can break it.
var _ = db.Declare(
	db.Fault(schema.OccurrenceCursorIndex,
		"an Automation's cursor is written by an upsert on this index: a Publish resets it, a tick moves it"),
	db.Fault("automation_occurrence_cursors_automation_id_key",
		"the Automation's one cursor, the same key as the Workspace's index, which the upsert writes on"),
)

// resetCursor starts the active span of the Automation's date Trigger at, and stands its watermark
// there: nothing before it is ever decided. The Workspace zone and the Contacts seen are forgotten,
// so the next tick reads the Workspace's every Contact over the span it decides.
func resetCursor(ctx context.Context, s *ent.Scoped, automationID int64, at time.Time) error {
	return s.AutomationOccurrenceCursor().Create().
		SetAutomationID(automationID).
		SetSpanStart(at).
		SetDecidedThrough(at).
		OnConflictColumns(automationoccurrencecursor.FieldWorkspaceID, automationoccurrencecursor.FieldAutomationID).
		Update(func(u *ent.AutomationOccurrenceCursorScopedUpsert) {
			u.SetSpanStart(at).SetDecidedThrough(at).ClearWorkspaceZone().ClearContactsSeenThrough()
		}).
		Exec(ctx)
}

// The 24 hours of ADR 0126: an occurrence the scheduler reaches later than this after its moment
// is `missed`, and a date set this long after its occurrence has none.
const lateBound = 24 * time.Hour

// changeOverlap is how far before the latest change a tick saw the next one reads the Contacts
// changed since: a transaction stamps a row's updated_at before it commits, so a change committed
// after the tick's read may carry an earlier stamp. A Contact read twice is decided once.
const changeOverlap = 10 * time.Minute

// candidatePage is how many candidates a band read loads at once: a page of Contacts, by id.
const candidatePage = 500

// DateDue returns the active, kept Automations whose live version has a date Trigger, across
// Workspaces, each with its id and its Workspace: the date scheduler's tick queues one job per
// Automation (ADR 0126). It reads with the raw client, as no Workspace scope covers every Workspace
// (docs/architecture/workspace-scoping.md).
func (m *Module) DateDue(ctx context.Context, client *ent.Client) ([]*ent.Automation, error) {
	return client.Automation.Query().
		Where(
			automation.StatusEQ(automation.StatusActive),
			automation.ArchivedAtIsNil(),
			automation.HasLiveVersionWith(automationversion.TriggerDateNotNil()),
		).
		Select(automation.FieldID, automation.FieldWorkspaceID).
		Order(ent.Asc(automation.FieldID)).
		All(ctx)
}

// window is a span of moments the scheduler decides: (from, to] for the occurrences it reaches,
// [from, to] for those of dates set late.
type window struct {
	from, to time.Time
	fromOpen bool
}

// holds reports whether the moment lies in the window.
func (w window) holds(at time.Time) bool {
	after := at.After(w.from) || !w.fromOpen && at.Equal(w.from)
	return after && !at.After(w.to)
}

// occurrenceCase is one occurrence the scheduler decides: the Automation and the live version it
// was found on, the Contact, the anniversary year, the window it must still fall in when the
// Contact is read under its lock, whether it is a date set late (never `missed`), the Workspace
// time zone and the Instance clock's now.
type occurrenceCase struct {
	automationID, versionID, contactID int64
	date                               datetrigger.Date
	year                               int
	window                             window
	late                               bool
	workspaceZone                      *string
	now                                time.Time
}

// DecideOccurrences decides the occurrences of the Automation's date Trigger the scheduler has not
// decided (ADR 0126), at the Instance clock's now, which every query takes as an argument:
//
//  1. those in (watermark, now]: one less than 24 hours old enrolls, an older one is `missed`;
//  2. those in [floor, watermark], floor the later of now − 24 hours and the start of the active
//     span, of the Contacts changed since the last tick, or of every Contact when the Workspace time
//     zone changed: dates set late, and occurrences a zone change moved back. They enroll and are
//     never `missed`;
//  3. the watermark moves to now.
//
// The candidates are the Contacts whose date falls in a band of calendar days around the window,
// read in Postgres; each one's moments come from the local calendar, and each occurrence is decided
// in a transaction of its own under the Contact's lock (decideOccurrence). An Automation that is
// not active, or whose live version has no date Trigger, is left alone, and the cursor moves only
// from where this run found it, so a Publish meanwhile, which resets it, wins.
func (m *StepExecution) DecideOccurrences(ctx context.Context, s *ent.Scoped, automationID int64) error {
	now := m.now()
	a, err := s.Automation().Query().Where(automation.ID(automationID)).WithLiveVersion().WithOccurrenceCursor().Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	live := a.Edges.LiveVersion
	if !active(a) || live == nil || live.TriggerDate == nil {
		return nil
	}
	cur := a.Edges.OccurrenceCursor
	if cur == nil {
		return resetCursor(ctx, s, a.ID, now)
	}
	watermark := cur.DecidedThrough
	if !now.After(watermark) {
		return nil
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return err
	}
	seen, err := latestChange(ctx, s)
	if err != nil {
		return err
	}
	base := occurrenceCase{automationID: a.ID, versionID: live.ID, date: *live.TriggerDate, workspaceZone: ws.Timezone, now: now}
	reached := base
	reached.window = window{from: watermark, to: now, fromOpen: true}
	if err := m.decideWindow(ctx, s, reached, nil); err != nil {
		return err
	}
	floor := later(now.Add(-lateBound), cur.SpanStart)
	if !floor.After(watermark) {
		late := base
		late.window, late.late = window{from: floor, to: watermark}, true
		var changedAfter *time.Time
		if cur.ContactsSeenThrough != nil && optional.Equal(cur.WorkspaceZone, ws.Timezone) {
			changedAfter = new(cur.ContactsSeenThrough.Add(-changeOverlap))
		}
		if err := m.decideWindow(ctx, s, late, changedAfter); err != nil {
			return err
		}
	}
	return s.AutomationOccurrenceCursor().Update().
		Where(
			automationoccurrencecursor.ID(cur.ID),
			automationoccurrencecursor.DecidedThrough(watermark),
			automationoccurrencecursor.SpanStart(cur.SpanStart),
		).
		SetDecidedThrough(now).
		SetNillableWorkspaceZone(ws.Timezone).
		SetNillableContactsSeenThrough(seen).
		Exec(ctx)
}

// later is the later of two moments.
func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// latestChange is the latest updated_at of the Workspace's Contacts, nil when it has none: the
// stamp, on the clock that writes the rows, the next tick reads the changes after. It reads the
// stamp alone, no Contact.
func latestChange(ctx context.Context, s *ent.Scoped) (*time.Time, error) {
	var last []struct {
		UpdatedAt time.Time `json:"updated_at"`
	}
	err := s.Contact().Query().Order(ent.Desc(contact.FieldUpdatedAt)).Limit(1).Select(contact.FieldUpdatedAt).Scan(ctx, &last)
	if err != nil || len(last) == 0 {
		return nil, err
	}
	return &last[0].UpdatedAt, nil
}

// decideWindow decides the occurrences of c's date in c's window of the Contacts that hold a key
// of its field in the band of the window, those changed after changedAfter only when it is given,
// a page at a time.
func (m *StepExecution) decideWindow(ctx context.Context, s *ent.Scoped, c occurrenceCase, changedAfter *time.Time) error {
	band := c.date.Band(c.window.from, c.window.to)
	var after int64
	for {
		page, err := candidates(ctx, s, c.date.Field, band, changedAfter, after)
		if err != nil {
			return err
		}
		for _, candidate := range page {
			after = candidate.ID
			years, err := yearsIn(c.date, c.window, candidate, c.workspaceZone)
			if err != nil {
				return fmt.Errorf("occurrences of contact %d: %w", candidate.ID, err)
			}
			for _, year := range years {
				one := c
				one.contactID, one.year = candidate.ID, year
				if err := m.decideOccurrence(ctx, s, one); err != nil {
					return fmt.Errorf("decide occurrence %d of contact %d: %w", year, candidate.ID, err)
				}
			}
		}
		if len(page) < candidatePage {
			return nil
		}
	}
}

// candidates is the page of the Contacts after the one after, by id, that hold a key of the field
// in the band (ContactDate, on its indexes), those changed after changedAfter only when it is
// given (on the Contacts' index of their change time).
func candidates(ctx context.Context, s *ent.Scoped, f datetrigger.Field, band datetrigger.Band, changedAfter *time.Time, after int64) ([]*ent.Contact, error) {
	// ent generates no range predicate of an edge's column: the keyset's is written with its builder.
	q := s.ContactDate().Query().
		Where(contactdate.KindEQ(f.Kind), contactdate.Key(f.Key), predicate.ContactDate(sql.FieldGT(contactdate.FieldContactID, after)))
	if first, last, ok := band.Range(); ok {
		q = q.Where(contactdate.DateGTE(first.Midnight()), contactdate.DateLTE(last.Midnight()))
	}
	if days, ok := band.MonthDays(); ok {
		q = q.Where(contactdate.MonthDayIn(days...))
	}
	if changedAfter != nil {
		q = q.Where(contactdate.HasContactWith(contact.WorkspaceID(s.WorkspaceID()), contact.UpdatedAtGT(*changedAfter)))
	}
	held, err := q.Order(ent.Asc(contactdate.FieldContactID)).Limit(candidatePage).WithContact().All(ctx)
	if err != nil {
		return nil, err
	}
	return lo.Map(held, func(d *ent.ContactDate, _ int) *ent.Contact { return d.Edges.Contact }), nil
}

// momentOf is the moment of the date's occurrence in the anniversary year for the Contact, read in
// its zone (the Workspace's when it has none, contactCalendar), false when it holds no date in the
// field or the year has no occurrence. An error is a date or a Workspace zone out of bounds.
func momentOf(d datetrigger.Date, c *ent.Contact, workspaceZone *string, year int) (time.Time, bool, error) {
	sched, err := d.Schedule()
	if err != nil {
		return time.Time{}, false, err
	}
	cal, err := contactCalendar(c, workspaceZone)
	if err != nil {
		return time.Time{}, false, err
	}
	date, ok := d.Field.Of(cal.Dates, cal.Workspace)
	if !ok {
		return time.Time{}, false, nil
	}
	at, ok := sched.Moment(date, year, cal.Zone)
	return at, ok, nil
}

// yearsIn are the anniversary years whose occurrence for the Contact falls in the window: an offset
// moves a year's moment into the calendar year before or after it at most.
func yearsIn(d datetrigger.Date, w window, c *ent.Contact, workspaceZone *string) ([]int, error) {
	var years []int
	for year := w.from.Year() - 1; year <= w.to.Year()+1; year++ {
		at, ok, err := momentOf(d, c, workspaceZone, year)
		if err != nil {
			return nil, err
		}
		if ok && w.holds(at) {
			years = append(years, year)
		}
	}
	return years, nil
}

// decideOccurrence decides one occurrence, once, in a transaction of its own under the Contact's
// lock (ADR 0126): it reads the Contact there and computes the moment again, and leaves it alone
// unless the Automation is still active on the version it was found on and the moment still falls
// in the window. An occurrence decided already (an Enrollment, or a skipped entry
// `active_enrollment` or `missed`) is left alone. One whose date was written through a Connection
// without mcp:send is skipped as `send_locked`, once, and stays undecided. One reached 24 hours
// late or more is `missed` when its date was written by its moment (decision 15 of #1613), and
// has none when the date was written after it; a date set late found by the late path is never
// `missed`. An Enrollment of the Contact in
// the Automation active at the moment skips it as `active_enrollment`; otherwise the Enrollment
// begins at the moment with an empty `event` (enroll).
func (m *StepExecution) decideOccurrence(ctx context.Context, s *ent.Scoped, c occurrenceCase) error {
	occurrence := c.date.Occurrence(c.year)
	o := origin{Cause: cause{occurrence: &occurrence}}
	_, err := m.start(ctx, s, func(ts *ent.Scoped, pub events.Publisher) (entered, error) {
		return m.enroll(ctx, ts, pub, c.automationID, c.contactID, o, func(a *ent.Automation, held *ent.Contact, o *origin) (bool, error) {
			live := a.Edges.LiveVersion
			if !active(a) || live.ID != c.versionID {
				return false, nil
			}
			at, ok, err := momentOf(c.date, held, c.workspaceZone, c.year)
			if err != nil || !ok || !c.window.holds(at) {
				return false, err
			}
			o.OccurredAt = at
			decided, err := occurrenceDecided(ctx, ts, a.ID, held.ID, occurrence)
			if err != nil || decided {
				return false, err
			}
			written, err := ts.ContactDate().Query().
				Where(contactdate.ContactID(held.ID), contactdate.KindEQ(c.date.Field.Kind), contactdate.Key(c.date.Field.Key)).
				Only(ctx)
			if err != nil {
				return false, err
			}
			if written.SendLocked {
				return false, recordSkip(ctx, ts, a.ID, held.ID, *o, automationskippedentry.ReasonSendLocked)
			}
			if c.late || c.now.Sub(at) < lateBound {
				return true, nil
			}
			if written.WrittenAt.After(at) {
				// A date written after its occurrence is a date set late (decision 15 of #1613), here
				// 24 hours late or more: it has no occurrence this year, and the app failed to send
				// nothing.
				return false, nil
			}
			return false, recordSkip(ctx, ts, a.ID, held.ID, *o, automationskippedentry.ReasonMissed)
		})
	})
	return err
}

// occurrenceDecided reports whether the Contact's occurrence of the Automation's date Trigger is
// decided: an Enrollment was made for it, or a skipped entry other than `send_locked` records it.
// It is read under the Contact's lock, which every decision of the occurrence takes, across the two
// tables each of whose partial unique indexes holds one decision.
func occurrenceDecided(ctx context.Context, s *ent.Scoped, automationID, contactID int64, occurrence int) (bool, error) {
	enrolled, err := s.AutomationRun().Query().
		Where(automationrun.AutomationID(automationID), automationrun.ContactID(contactID), automationrun.Occurrence(occurrence)).
		Exist(ctx)
	if err != nil || enrolled {
		return enrolled, err
	}
	return s.AutomationSkippedEntry().Query().
		Where(
			automationskippedentry.AutomationID(automationID),
			automationskippedentry.ContactID(contactID),
			automationskippedentry.Occurrence(occurrence),
			automationskippedentry.ReasonNEQ(automationskippedentry.ReasonSendLocked),
		).
		Exist(ctx)
}
