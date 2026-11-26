package automations

import (
	"context"
	"fmt"
	"slices"
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
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
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

// bandMargin is how many calendar days the band of a window reaches past it on each side: a local
// day lies within 14 hours of UTC, and a datetime value with an offset falls on its own date or the
// day either side of it in the Workspace time zone.
const bandMargin = 3

// maxBandDays is the widest band narrowed by its days: a window longer than a year reads every
// Contact that holds the field.
const maxBandDays = 400

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
// stamp, on the clock that writes the rows, the next tick reads the changes after.
func latestChange(ctx context.Context, s *ent.Scoped) (*time.Time, error) {
	last, err := s.Contact().Query().Order(ent.Desc(contact.FieldUpdatedAt)).Select(contact.FieldUpdatedAt).First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &last.UpdatedAt, nil
}

// decideWindow decides the occurrences of c's date in c's window of the Contacts in the band of the
// window (datePredicate), those changed after changedAfter only when it is given.
func (m *StepExecution) decideWindow(ctx context.Context, s *ent.Scoped, c occurrenceCase, changedAfter *time.Time) error {
	q := s.Contact().Query().Where(datePredicate(c.date, c.window))
	if changedAfter != nil {
		q = q.Where(contact.UpdatedAtGT(*changedAfter))
	}
	candidates, err := q.Order(ent.Asc(contact.FieldID)).All(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		for _, year := range yearsIn(c.date, c.window, candidate, c.workspaceZone) {
			one := c
			one.contactID, one.year = candidate.ID, year
			if err := m.decideOccurrence(ctx, s, one); err != nil {
				return fmt.Errorf("decide occurrence %d of contact %d: %w", year, candidate.ID, err)
			}
		}
	}
	return nil
}

// momentOf is the moment of the date's occurrence in the anniversary year for the Contact, read in
// its zone (the Workspace's when it has none), false when it holds no date in the field or the
// year has no occurrence.
func momentOf(d datetrigger.Date, c *ent.Contact, workspaceZone *string, year int) (time.Time, bool) {
	sched, err := d.Schedule()
	if err != nil {
		return time.Time{}, false
	}
	wsLoc, err := localcalendar.Zone(nil, workspaceZone)
	if err != nil {
		return time.Time{}, false
	}
	date, ok := d.Field.Of(datetrigger.Holder{Birthday: c.Birthday, CustomFields: c.CustomFields}, wsLoc)
	if !ok {
		return time.Time{}, false
	}
	loc, err := localcalendar.Zone(c.TimeZone, workspaceZone)
	if err != nil {
		return time.Time{}, false
	}
	return sched.Moment(date, year, loc)
}

// yearsIn are the anniversary years whose occurrence for the Contact falls in the window: an offset
// moves a year's moment into the calendar year before or after it at most.
func yearsIn(d datetrigger.Date, w window, c *ent.Contact, workspaceZone *string) []int {
	var years []int
	for year := w.from.Year() - 1; year <= w.to.Year()+1; year++ {
		if at, ok := momentOf(d, c, workspaceZone, year); ok && w.holds(at) {
			years = append(years, year)
		}
	}
	return years
}

// decideOccurrence decides one occurrence, once, in a transaction of its own under the Contact's
// lock (ADR 0126): it reads the Contact there and computes the moment again, and leaves it alone
// unless the Automation is still active on the version it was found on and the moment still falls
// in the window. An occurrence decided already (an Enrollment, or a skipped entry
// `active_enrollment` or `missed`) is left alone. One whose date was last written through a
// Connection without mcp:send is skipped as `send_locked`, once, and stays undecided; one reached
// 24 hours late or more is `missed`, unless it is a date set late. An Enrollment of the Contact in
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
			at, ok := momentOf(c.date, held, c.workspaceZone, c.year)
			if !ok || !c.window.holds(at) {
				return false, nil
			}
			o.OccurredAt = at
			decided, err := occurrenceDecided(ctx, ts, a.ID, held.ID, occurrence)
			if err != nil || decided {
				return false, err
			}
			switch {
			case slices.Contains(held.SendLockedDates, c.date.Field):
				return false, recordSkip(ctx, ts, a.ID, held.ID, *o, automationskippedentry.ReasonSendLocked)
			case !c.late && c.now.Sub(at) >= lateBound:
				return false, recordSkip(ctx, ts, a.ID, held.ID, *o, automationskippedentry.ReasonMissed)
			}
			return true, nil
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

// datePredicate narrows the Contacts to those whose date in the field may have an occurrence in the
// window: the calendar days of the band (bandDays) its date falls on, by month and day for a yearly
// date and in full for a once date. 29 February goes with 28 February, its day in a common year.
// A band too wide to list reads every Contact that holds the field.
func datePredicate(d datetrigger.Date, w window) predicate.Contact {
	days, narrowed := bandDays(d, w)
	layout := "01-02"
	if d.Recurrence == localcalendar.Once {
		layout = time.DateOnly
	}
	var values []any
	for _, day := range days {
		values = append(values, day.Format(layout))
		if d.Recurrence != localcalendar.Once && day.Month() == time.February && day.Day() == 28 {
			values = append(values, "02-29")
		}
	}
	values = lo.Uniq(values)
	return func(sel *sql.Selector) {
		// raw sql: ent has no expression of a date's month and day, nor of a text property of a jsonb
		// column; the format, the key and the days are bound arguments, the substring's bounds
		// constants of this function.
		held := sql.P(func(b *sql.Builder) {
			switch d.Field.Kind {
			case datetrigger.Birthday:
				b.WriteString("to_char(").WriteString(sel.C(contact.FieldBirthday)).WriteString(", ")
				b.Arg(pgDateFormat(d.Recurrence)).WriteString(")")
			case datetrigger.CustomField:
				b.WriteString("substring(").WriteString(sel.C(contact.FieldCustomFields)).WriteString(" ->> ").Arg(d.Field.Key)
				b.WriteString(fmt.Sprintf(" from %d for %d)", pgDateStart(d.Recurrence), len(layout)))
			}
			if !narrowed {
				b.WriteString(" IS NOT NULL")
				return
			}
			b.WriteString(" IN (").Args(values...).WriteString(")")
		})
		sel.Where(held)
	}
}

// pgDateFormat is the Postgres format of a birthday's text the band compares: its month and day for
// a yearly date, the whole date for a once date.
func pgDateFormat(r localcalendar.Recurrence) string {
	if r == localcalendar.Once {
		return "YYYY-MM-DD"
	}
	return "MM-DD"
}

// pgDateStart is where, in a datetime value's ISO text, the part the band compares starts: its
// month for a yearly date, its year for a once date.
func pgDateStart(r localcalendar.Recurrence) int {
	if r == localcalendar.Once {
		return 1
	}
	return len("2006-") + 1
}

// bandDays are the calendar days a field's date may fall on for an occurrence in the window: the
// days of the window, bandMargin each side, moved back by the offset; false when they are more
// than maxBandDays, which narrows nothing.
func bandDays(d datetrigger.Date, w window) ([]time.Time, bool) {
	first := w.from.UTC().Truncate(24*time.Hour).AddDate(0, 0, -bandMargin)
	last := w.to.UTC().Truncate(24*time.Hour).AddDate(0, 0, bandMargin)
	if last.Sub(first) > maxBandDays*24*time.Hour {
		return nil, false
	}
	var days []time.Time
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		days = append(days, day.AddDate(0, 0, -d.OffsetDays))
	}
	return days, true
}
