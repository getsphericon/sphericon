// The local calendar's vocabulary (ADR 0126): a local time of day and the days of the week, which
// a Wait until reads in the Contact's time zone.

namespace Sphericon;

/** A local time of day, `HH:MM` in whole minutes from 00:00 to 23:59, read in a time zone the
 * place that holds it names (`09:00`, `18:30`) */
@pattern("^([01]\\d|2[0-3]):[0-5]\\d$")
scalar LocalTime extends string;

/** A day of the week, by its ISO name in lower case */
enum Weekday {
  monday,
  tuesday,
  wednesday,
  thursday,
  friday,
  saturday,
  sunday,
}

/** The Contact's date of birth, a date field every Contact has */
model BirthdayDateField {
  kind: "birthday";
}

/** A datetime Custom field of the Workspace, its key in a field of its own. Its value's calendar
 * date is read in the Workspace time zone: a value with an offset is moved into it, one without an
 * offset or a bare date keeps its own date. */
model CustomFieldDateField {
  kind: "customField";

  /** The Custom field's key */
  key: string;
}

model BirthdayDateFieldInput is BirthdayDateField;
model CustomFieldDateFieldInput is CustomFieldDateField;

/** A date a Contact holds, by `kind`: its `birthday`, or a datetime Custom field */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union DateField {
  birthday: BirthdayDateField,
  customField: CustomFieldDateField,
}

/** A date a Contact holds, by `kind`, as a request writes it */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union DateFieldInput {
  birthday: BirthdayDateFieldInput,
  customField: CustomFieldDateFieldInput,
}

/** How often a date comes: `yearly` on each anniversary, from the year after the date's own;
 * `once` on the date itself. The birthday is always past, so it is only yearly. */
enum DateRecurrence {
  yearly,
  once,
}

/** A whole number of days before (negative) or after the date, within a year */
@minValue(-365)
@maxValue(365)
scalar DateOffsetDays extends int32;
