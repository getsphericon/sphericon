package automations_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The date scheduler (ADR 0126), worked under the harness clock: an Automation's occurrences are
// decided by DecideOccurrences, the job its tick queues every minute. Alice has no time zone (the
// Workspace's, UTC while Acme sets none) and the export subject lives in Berlin.

// dateCase is an Instance whose clock a test moves, and its modules.
type dateCase struct {
	t        *testing.T
	env      *testhelper.TestEnv
	clock    *testhelper.Clock
	acme     *ent.Scoped
	module   *automations.Module
	steps    *automations.StepExecution
	contacts *contacts.Module
}

func newDateCase(t *testing.T, at time.Time) *dateCase {
	t.Helper()
	clock := testhelper.NewClock(at)
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	return &dateCase{
		t: t, env: env, clock: clock, acme: env.DB.Scoped(fixtures.AcmeID),
		module: testhelper.Module[*automations.Module](env), steps: testhelper.Module[*automations.StepExecution](env),
		contacts: testhelper.Module[*contacts.Module](env),
	}
}

// at is a moment in UTC.
func at(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

// birthday is a yearly date Trigger on the birthday, offset days from it, at the local time.
func birthday(offset int, localTime string) datetrigger.Date {
	return datetrigger.Date{Field: datetrigger.Field{Kind: datetrigger.Birthday}, Recurrence: localcalendar.Yearly, OffsetDays: offset, LocalTime: localTime}
}

// publish makes and publishes, at the clock's now, an Automation on the date whose one Step, a
// wait of a year and a half, keeps each Enrollment active, and returns its id.
func (c *dateCase) publish(d datetrigger.Date) int64 {
	c.t.Helper()
	ctx := c.t.Context()
	created, err := c.module.Create(ctx, c.acme, automations.CreateInput{
		Name: "On a date", Trigger: automations.Trigger{Date: &d},
		Steps: new(automationstep.Root(automationstep.New(automationstep.Wait{Seconds: 540 * 24 * 3600}))),
	})
	require.NoError(c.t, err)
	c.republish(created.ID)
	return created.ID
}

// republish publishes the Automation again at the clock's now.
func (c *dateCase) republish(id int64) {
	c.t.Helper()
	ctx := c.t.Context()
	_, err := c.module.Publish(ctx, c.acme, id, testhelper.AutomationRevisionIn(ctx, c.t, c.acme, id))
	require.NoError(c.t, err)
}

// tickAt moves the clock to the moment and runs the scheduler's decision of the Automation.
func (c *dateCase) tickAt(id int64, now time.Time) {
	c.t.Helper()
	c.clock.Set(now)
	require.NoError(c.t, c.steps.DecideOccurrences(c.t.Context(), c.acme, id))
}

// write changes the Contact through the contacts module, in ctx: its Core and, when given, its
// Custom fields.
func (c *dateCase) write(ctx context.Context, contactID int64, core contacts.Core, fields map[string]any) {
	c.t.Helper()
	held, err := c.env.DB.Contact.Get(ctx, contactID)
	require.NoError(c.t, err)
	in := contacts.UpdateInput{Core: core, Revision: held.Revision}
	if fields != nil {
		in.CustomFields = patch.Set(fields)
	}
	_, err = c.contacts.Update(ctx, c.acme, contactID, in)
	require.NoError(c.t, err)
}

// born sets the Contact's birthday and time zone (none when empty), as an /api write does.
func (c *dateCase) born(contactID int64, day time.Time, zone string) {
	c.t.Helper()
	tz := patch.Clear[string]()
	if zone != "" {
		tz = patch.Set(zone)
	}
	c.write(c.t.Context(), contactID, contacts.Core{Birthday: patch.Set(day), TimeZone: tz}, nil)
}

// beganAt is when each of the Contact's Enrollments in the Automation began, in UTC.
func (c *dateCase) beganAt(id, contactID int64) []time.Time {
	c.t.Helper()
	return lo.Map(runsOf(c.t, c.env, id, contactID), func(r *ent.AutomationRun, _ int) time.Time { return r.BeganAt.UTC() })
}

// skips is why each of the Contact's occurrences in the Automation was skipped.
func (c *dateCase) skips(id, contactID int64) []automationskippedentry.Reason {
	c.t.Helper()
	return lo.Map(skippedOf(c.t, c.env, id, contactID), func(e *ent.AutomationSkippedEntry, _ int) automationskippedentry.Reason { return e.Reason })
}

// A yearly birthday Trigger at 10:00 enrolls a Berlin Contact at 10:00 Berlin time and a Contact
// with no zone at 10:00 in the Workspace's, each Enrollment beginning then with no triggering Event.
func TestADateTriggerEnrollsAtTheLocalTimeOfEachContact(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	c.born(fixtures.ContactExportSubjectID, at(1990, time.March, 14, 0, 0), "Europe/Berlin")
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	id := c.publish(birthday(0, "10:00"))

	c.tickAt(id, at(2027, time.March, 14, 9, 30))
	berlinFirst, aliceFirst := c.beganAt(id, fixtures.ContactExportSubjectID), c.beganAt(id, fixtures.ContactAliceID)
	c.tickAt(id, at(2027, time.March, 14, 10, 1))
	runs := runsOf(t, c.env, id, fixtures.ContactAliceID)

	assert.Equal(t, []time.Time{at(2027, time.March, 14, 9, 0)}, berlinFirst, "10:00 CET")
	assert.Empty(t, aliceFirst)
	assert.Equal(t, []time.Time{at(2027, time.March, 14, 10, 0)}, c.beganAt(id, fixtures.ContactAliceID))
	require.Len(t, runs, 1)
	assert.Nil(t, runs[0].EventAction, "no Event began it")
	assert.Empty(t, runs[0].EventProperties)
}

// "3 days before" a 2 January birthday enrolls on 30 December, for the new anniversary year, and a
// second tick that day enrolls no one.
func TestADateTriggerCountsTheAnniversaryYearOfTheDateTheOffsetCountsFrom(t *testing.T) {
	c := newDateCase(t, at(2026, time.December, 29, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.January, 2, 0, 0), "")
	id := c.publish(birthday(-3, "10:00"))

	c.tickAt(id, at(2026, time.December, 30, 10, 5))
	c.tickAt(id, at(2026, time.December, 30, 18, 0))
	runs := runsOf(t, c.env, id, fixtures.ContactAliceID)

	require.Len(t, runs, 1)
	assert.Equal(t, at(2026, time.December, 30, 10, 0), runs[0].BeganAt.UTC())
	assert.Equal(t, new(2027), runs[0].Occurrence)
}

// A Contact enrolled for this year whose birthday is then moved to a later day this year is not
// enrolled again this year.
func TestADateCorrectedAfterItsYearEnrolledEnrollsNoOneAgain(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	id := c.publish(birthday(0, "10:00"))
	c.tickAt(id, at(2027, time.March, 14, 10, 5))

	c.born(fixtures.ContactAliceID, at(1990, time.March, 20, 0, 0), "")
	c.tickAt(id, at(2027, time.March, 20, 10, 5))

	assert.Equal(t, []time.Time{at(2027, time.March, 14, 10, 0)}, c.beganAt(id, fixtures.ContactAliceID))
}

// An occurrence while last year's Enrollment is still active is skipped as `active_enrollment`;
// once it has ended, the next anniversary year enrolls the Contact again.
func TestAnOccurrenceDuringLastYearsEnrollmentIsSkippedUntilItEnds(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	id := c.publish(birthday(0, "10:00"))
	c.tickAt(id, at(2027, time.March, 14, 10, 5))

	c.tickAt(id, at(2028, time.March, 14, 10, 5))
	skipped := c.skips(id, fixtures.ContactAliceID)
	var exited bool
	err := c.env.Bus.WithinScopedTx(t.Context(), c.acme, func(ts *ent.Scoped, _ events.Publisher) (err error) {
		exited, err = c.module.Exit(t.Context(), ts, id, fixtures.ContactAliceID)
		return err
	})
	require.NoError(t, err)
	c.tickAt(id, at(2029, time.March, 14, 10, 5))

	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonActiveEnrollment}, skipped)
	assert.True(t, exited)
	assert.Equal(t, []time.Time{at(2027, time.March, 14, 10, 0), at(2029, time.March, 14, 10, 0)}, c.beganAt(id, fixtures.ContactAliceID))
}

// A once Trigger on a datetime Custom field enrolls the Contact once: a later tick, and a corrected
// date still ahead, enroll no one again.
func TestAOnceDateEnrollsOnce(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 19, 12, 0))
	c.write(t.Context(), fixtures.ContactAliceID, contacts.Core{}, map[string]any{fixtures.CustomFieldAcmeTrialEndsAtKey: "2027-03-20T00:00:00Z"})
	id := c.publish(datetrigger.Date{
		Field:      datetrigger.Field{Kind: datetrigger.CustomField, Key: fixtures.CustomFieldAcmeTrialEndsAtKey},
		Recurrence: localcalendar.Once, LocalTime: "10:00",
	})

	c.tickAt(id, at(2027, time.March, 20, 10, 5))
	c.tickAt(id, at(2027, time.March, 21, 10, 5))
	c.write(t.Context(), fixtures.ContactAliceID, contacts.Core{}, map[string]any{fixtures.CustomFieldAcmeTrialEndsAtKey: "2027-04-01"})
	c.tickAt(id, at(2027, time.April, 1, 10, 5))

	assert.Equal(t, []time.Time{at(2027, time.March, 20, 10, 0)}, c.beganAt(id, fixtures.ContactAliceID))
}

// A Contact's time zone changed before today's occurrence moves it to the new zone's local time,
// and a Workspace time zone change moves the occurrences of the Contacts without one.
func TestATimeZoneChangeMovesTheOccurrencesNotReached(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	c.born(fixtures.ContactExportSubjectID, at(1990, time.March, 14, 0, 0), "Europe/Berlin")
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	id := c.publish(birthday(0, "10:00"))
	c.tickAt(id, at(2027, time.March, 13, 20, 0))

	c.born(fixtures.ContactExportSubjectID, at(1990, time.March, 14, 0, 0), "Asia/Tokyo")
	ws, err := c.acme.Workspace(t.Context())
	require.NoError(t, err)
	_, err = testhelper.Module[*workspaces.Module](c.env).SetTimezone(t.Context(), c.acme, ws.Revision, "America/New_York")
	require.NoError(t, err)
	c.tickAt(id, at(2027, time.March, 14, 1, 5))
	c.tickAt(id, at(2027, time.March, 14, 14, 5))

	assert.Equal(t, []time.Time{at(2027, time.March, 14, 1, 0)}, c.beganAt(id, fixtures.ContactExportSubjectID), "10:00 in Tokyo")
	assert.Equal(t, []time.Time{at(2027, time.March, 14, 14, 0)}, c.beganAt(id, fixtures.ContactAliceID), "10:00 EDT")
}

// Publishing after an occurrence earlier the same day enrolls no one for it, and deactivating across
// an occurrence and publishing again enrolls no one for it and counts it as no `missed`.
func TestOnlyOccurrencesOfTheActiveSpanEnroll(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 14, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	id := c.publish(birthday(0, "10:00"))
	c.tickAt(id, at(2027, time.March, 14, 12, 5))
	afterPublish := c.beganAt(id, fixtures.ContactAliceID)

	_, err := c.module.Deactivate(t.Context(), c.acme, id)
	require.NoError(t, err)
	c.clock.Set(at(2028, time.March, 14, 12, 0))
	c.republish(id)
	c.tickAt(id, at(2028, time.March, 14, 12, 5))

	assert.Empty(t, afterPublish)
	assert.Empty(t, c.beganAt(id, fixtures.ContactAliceID))
	assert.Empty(t, c.skips(id, fixtures.ContactAliceID))
}

// An occurrence the scheduler reaches 30 hours late is `missed`; 20 hours late it enrolls.
func TestAnOccurrenceReachedLateEnrollsWithin24Hours(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	c.born(fixtures.ContactExportSubjectID, at(1990, time.March, 14, 0, 0), "Pacific/Honolulu")
	id := c.publish(birthday(0, "10:00"))

	c.tickAt(id, at(2027, time.March, 15, 16, 0))

	assert.Empty(t, c.beganAt(id, fixtures.ContactAliceID))
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonMissed}, c.skips(id, fixtures.ContactAliceID), "30 hours late")
	assert.Equal(t, []time.Time{at(2027, time.March, 14, 20, 0)}, c.beganAt(id, fixtures.ContactExportSubjectID), "10:00 in Honolulu, 20 hours late")
}

// A date set two hours after its 10:00 enrolls the Contact by the late path; one set 30 hours after
// has no occurrence this year, and counts no `missed`.
func TestADateSetLateEnrollsWithin24Hours(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	id := c.publish(birthday(0, "10:00"))
	c.tickAt(id, at(2027, time.March, 14, 11, 59))

	c.clock.Set(at(2027, time.March, 14, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	c.tickAt(id, at(2027, time.March, 14, 12, 1))
	c.tickAt(id, at(2027, time.March, 15, 15, 59))
	c.born(fixtures.ContactExportSubjectID, at(1990, time.March, 14, 0, 0), "")
	c.tickAt(id, at(2027, time.March, 15, 16, 1))

	assert.Equal(t, []time.Time{at(2027, time.March, 14, 10, 0)}, c.beganAt(id, fixtures.ContactAliceID))
	assert.Empty(t, c.beganAt(id, fixtures.ContactExportSubjectID))
	assert.Empty(t, c.skips(id, fixtures.ContactExportSubjectID))
}

// 29 February enrolls on 28 February in a common year, and 02:30 on the day the clocks spring
// forward enrolls at 03:00, the first moment after the gap.
func TestADateTriggerKeepsTheCalendarRules(t *testing.T) {
	c := newDateCase(t, at(2027, time.February, 27, 12, 0))
	c.born(fixtures.ContactAliceID, at(1992, time.February, 29, 0, 0), "")
	c.born(fixtures.ContactExportSubjectID, at(1990, time.March, 28, 0, 0), "Europe/Berlin")
	leap := c.publish(birthday(0, "10:00"))
	gap := c.publish(birthday(0, "02:30"))

	c.tickAt(leap, at(2027, time.February, 28, 10, 5))
	c.tickAt(gap, at(2027, time.March, 28, 1, 5))

	assert.Equal(t, []time.Time{at(2027, time.February, 28, 10, 0)}, c.beganAt(leap, fixtures.ContactAliceID))
	assert.Equal(t, []time.Time{at(2027, time.March, 28, 1, 0)}, c.beganAt(gap, fixtures.ContactExportSubjectID), "03:00 CEST")
}

// A birthday set to today through a Connection without mcp:send enrolls no one at its occurrence
// and records `send_locked`; the same value written again without the mark changes nothing, and a
// new value written without it within 24 hours after the occurrence enrolls it.
func TestASendLockedDateEnrollsNoOneUntilAWriteWithoutTheMark(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	id := c.publish(birthday(0, "10:00"))
	locked := events.SendLocked(t.Context())
	c.write(locked, fixtures.ContactAliceID, contacts.Core{Birthday: patch.Set(at(1990, time.March, 14, 0, 0))}, nil)

	c.tickAt(id, at(2027, time.March, 14, 10, 5))
	held := c.skips(id, fixtures.ContactAliceID)
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	c.tickAt(id, at(2027, time.March, 14, 12, 0))
	same := c.beganAt(id, fixtures.ContactAliceID)
	c.born(fixtures.ContactAliceID, at(1991, time.March, 14, 0, 0), "")
	c.tickAt(id, at(2027, time.March, 14, 14, 0))

	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonSendLocked}, held)
	assert.Empty(t, same, "the same value written again lifts nothing")
	assert.Equal(t, []time.Time{at(2027, time.March, 14, 10, 0)}, c.beganAt(id, fixtures.ContactAliceID))
}

// A date written during a scheduler outage more than 24 hours after its occurrence is a date set
// late (decision 15 of #1613): it has no occurrence this year and counts no `missed`, while a date
// in place at its occurrence is `missed`.
func TestADateWrittenDuringAnOutageAfterItsOccurrenceIsNotMissed(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	c.born(fixtures.ContactExportSubjectID, at(1990, time.March, 14, 0, 0), "")
	id := c.publish(birthday(0, "10:00"))
	c.tickAt(id, at(2027, time.March, 13, 12, 5))

	c.clock.Set(at(2027, time.March, 15, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	c.tickAt(id, at(2027, time.March, 15, 16, 0))

	assert.Empty(t, c.beganAt(id, fixtures.ContactAliceID))
	assert.Empty(t, c.skips(id, fixtures.ContactAliceID), "written 26 hours after its occurrence")
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonMissed}, c.skips(id, fixtures.ContactExportSubjectID))
}

// After an outage of years each year's occurrence is decided `missed` once, and a tick that comes
// round again records none twice.
func TestEveryOccurrenceOfAnOutageIsMissedOnce(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	id := c.publish(birthday(0, "10:00"))

	c.tickAt(id, at(2034, time.March, 15, 12, 0))
	c.tickAt(id, at(2034, time.March, 15, 12, 5))

	assert.Equal(t, slices.Repeat([]automationskippedentry.Reason{automationskippedentry.ReasonMissed}, 8), c.skips(id, fixtures.ContactAliceID))
	assert.Empty(t, c.beganAt(id, fixtures.ContactAliceID))
}

// A time zone change that moves an occurrence not yet reached to a moment already past, eastward,
// enrolls it by the late path and counts no `missed` (decision 12 of #1613): the Workspace's, for a
// Contact without a zone, as the Contact's own.
func TestAnOccurrenceAZoneChangeMovesIntoThePastEnrollsLate(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	c.born(fixtures.ContactExportSubjectID, at(1990, time.March, 14, 0, 0), "Europe/London")
	id := c.publish(birthday(0, "10:00"))
	c.tickAt(id, at(2027, time.March, 14, 5, 0))

	ws, err := c.acme.Workspace(t.Context())
	require.NoError(t, err)
	_, err = testhelper.Module[*workspaces.Module](c.env).SetTimezone(t.Context(), c.acme, ws.Revision, "Asia/Tokyo")
	require.NoError(t, err)
	c.born(fixtures.ContactExportSubjectID, at(1990, time.March, 14, 0, 0), "Asia/Kolkata")
	c.tickAt(id, at(2027, time.March, 14, 5, 5))

	assert.Equal(t, []time.Time{at(2027, time.March, 14, 1, 0)}, c.beganAt(id, fixtures.ContactAliceID), "10:00 in Tokyo")
	assert.Equal(t, []time.Time{at(2027, time.March, 14, 4, 30)}, c.beganAt(id, fixtures.ContactExportSubjectID), "10:00 in Kolkata")
	assert.Empty(t, c.skips(id, fixtures.ContactAliceID))
	assert.Empty(t, c.skips(id, fixtures.ContactExportSubjectID))
}

// contactsRead records the id of every Contact a query of the Instance returns from now on.
func (c *dateCase) contactsRead() *[]int64 {
	c.t.Helper()
	read := new([]int64)
	c.env.DB.Contact.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			v, err := next.Query(ctx, q)
			rows, _ := v.([]*ent.Contact)
			*read = append(*read, lo.Map(rows, func(r *ent.Contact, _ int) int64 { return r.ID })...)
			return v, err
		})
	}))
	return read
}

// The scheduler reads only the Contacts whose date falls in the band of its window: a birthday
// half a year away is never read, on the path of the occurrences reached and on the path of the
// dates set late, while today's enrolls.
func TestADateFarOutsideTheWindowIsNeverRead(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.March, 14, 0, 0), "")
	c.born(fixtures.ContactExportSubjectID, at(1990, time.September, 14, 0, 0), "")
	id := c.publish(birthday(0, "10:00"))
	read := c.contactsRead()

	c.tickAt(id, at(2027, time.March, 14, 10, 5))
	c.tickAt(id, at(2027, time.March, 14, 10, 6))

	assert.Contains(t, *read, int64(fixtures.ContactAliceID))
	assert.NotContains(t, *read, int64(fixtures.ContactExportSubjectID))
	assert.Equal(t, []time.Time{at(2027, time.March, 14, 10, 0)}, c.beganAt(id, fixtures.ContactAliceID))
}

// Every writer of a Contact keeps the dates the scheduler finds it by: a Contact created with a
// birthday, one an Identify creates with it and one an Identify gives it are enrolled, and one whose
// birthday is cleared is not.
func TestADateEveryContactWriterSetsIsFound(t *testing.T) {
	c := newDateCase(t, at(2027, time.March, 13, 12, 0))
	ctx := t.Context()
	day := at(1990, time.March, 14, 0, 0)
	created, err := c.contacts.Create(ctx, c.acme, contact.SourceAPI, contacts.CreateInput{Email: new("born@example.com"), Birthday: &day})
	require.NoError(t, err)
	identified, err := c.contacts.Upsert(ctx, c.acme, contact.SourceAPI, contacts.UpsertInput{Email: new("identified@example.com"), Birthday: patch.Set(day)})
	require.NoError(t, err)
	enriched, err := c.contacts.Upsert(ctx, c.acme, contact.SourceAPI, contacts.UpsertInput{Email: new(fixtures.ContactBobEmail), Birthday: patch.Set(day)})
	require.NoError(t, err)
	c.born(fixtures.ContactAliceID, day, "")
	c.write(ctx, fixtures.ContactAliceID, contacts.Core{Birthday: patch.Clear[time.Time]()}, nil)
	id := c.publish(birthday(0, "10:00"))

	c.tickAt(id, at(2027, time.March, 14, 10, 5))

	began := []time.Time{at(2027, time.March, 14, 10, 0)}
	assert.Equal(t, began, c.beganAt(id, created.ID), "created")
	assert.Equal(t, began, c.beganAt(id, identified.Contact.ID), "created by an Identify")
	assert.Equal(t, began, c.beganAt(id, enriched.Contact.ID), "given it by an Identify")
	assert.Equal(t, int64(fixtures.ContactBobID), enriched.Contact.ID)
	assert.Empty(t, c.beganAt(id, fixtures.ContactAliceID), "cleared")
}

// The report counts a date Trigger's occurrences as it counts any entry (ADR 0126): one tick that
// enrolls Carol, skips Bob as `active_enrollment` (last year's Enrollment still waits), finds
// Alice's occurrence 30 hours late and holds the export subject's send-locked date shows each, the
// Enrollment entered and each skip by its reason. The fixture birthdays of Liam and Emma fall
// outside the period.
func TestTheReportCountsTheOccurrencesOfADateTriggerByOutcome(t *testing.T) {
	c := newDateCase(t, at(2026, time.September, 8, 12, 0))
	c.born(fixtures.ContactBobID, at(1990, time.September, 10, 0, 0), "")
	id := c.publish(birthday(0, "10:00"))
	c.tickAt(id, at(2026, time.September, 10, 10, 5))
	c.clock.Set(at(2026, time.September, 20, 12, 0))
	c.born(fixtures.ContactAliceID, at(1990, time.September, 9, 0, 0), "")
	c.born(fixtures.ContactCarolID, at(1990, time.September, 10, 0, 0), "")
	c.write(events.SendLocked(t.Context()), fixtures.ContactExportSubjectID,
		contacts.Core{Birthday: patch.Set(at(1990, time.September, 10, 0, 0)), TimeZone: patch.Clear[string]()}, nil)

	c.tickAt(id, at(2027, time.September, 10, 16, 0))
	report, err := c.module.ReportOf(t.Context(), c.acme, id, automations.ReportPeriod7Days)
	require.NoError(t, err)

	assert.Equal(t, 1, report.Results.Current.Entered)
	assert.Equal(t, 1, report.Results.Current.Contacts)
	assert.Equal(t, automations.SkippedCounts{ActiveEnrollment: 1, Missed: 1, SendLocked: 1}, report.Results.Current.Skipped)
}
