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
