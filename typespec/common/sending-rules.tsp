// The Workspace's sending rules (ADR 0134, ADR 0137): its Frequency caps and its Sending windows.
// /site reads and replaces them; /api and MCP read them with the Workspace and change nothing.

import "./local-calendar.tsp";

namespace Sphericon;

/** A Channel a Frequency cap counts marketing messages on (ADR 0137) */
enum FrequencyCapChannel {
  email,
  sms,
  telegram,
}

/** A Channel a Sending window holds (ADR 0134): only SMS, since email and Telegram go out at any
 * hour */
enum SendingWindowChannel {
  sms,
}

/** The most marketing messages one Destination receives in a Frequency cap's period */
@minValue(1)
@maxValue(1000)
scalar FrequencyCapMaxMessages extends int32;

/** A Frequency cap's rolling period in minutes: one minute to seven days */
@minValue(1)
@maxValue(10080)
scalar FrequencyCapPeriodMinutes extends int32;

/** A minute of the day: 0 is midnight at its start, 1440 midnight at its end */
@minValue(0)
@maxValue(1440)
scalar MinuteOfDay extends int32;

/** One of the Workspace's Frequency caps (ADR 0137): at most `maxMessages` marketing messages to one
 * Destination on the Channel per rolling `periodMinutes`. A cap with a country is for the numbers of
 * that country and outranks the Channel's own; one with no maximum removes the cap there. Every
 * Broadcast and Automation message counts; Transactional messages and Test sends do not. */
model FrequencyCap {
  /** The Channel it counts on */
  channel: FrequencyCapChannel;

  /** The country of the numbers it is for, on a phone Channel only; null for every Destination on
   * the Channel */
  country: CountryCode | null;

  /** The most marketing messages one Destination receives in the period; null removes the cap */
  maxMessages: FrequencyCapMaxMessages | null;

  /** The rolling period in minutes; null exactly when the maximum is */
  periodMinutes: FrequencyCapPeriodMinutes | null;
}

/** The hours a Sending window is open on one day of the week, in the recipient's local time */
model SendingWindowHours {
  /** The day, in the recipient's local time */
  day: Weekday;

  /** The minute of the day it opens */
  opensAt: MinuteOfDay;

  /** The minute of the day it closes, after it opens */
  closesAt: MinuteOfDay;
}

/** One of the Workspace's Sending windows (ADR 0134): the hours of each day of the week, in the
 * recipient's local time, in which a marketing message on the Channel may go out; a day it does not
 * list stays closed. A window with a country is for the numbers of that country and outranks the
 * Channel's own. It opens at least once a week. */
model SendingWindow {
  /** The Channel it holds */
  channel: SendingWindowChannel;

  /** The country of the numbers it is for; null for every number on the Channel */
  country: CountryCode | null;

  /** The hours it is open, at most one entry per day */
  hours: SendingWindowHours[];
}

/** The Workspace's sending rules: its Frequency caps and its Sending windows. A new Workspace has
 * three marketing texts per 1440 minutes on SMS and an SMS window of 8:00 to 20:00 every day. The
 * Workspace answers for its compliance: the app keeps no table of laws, and the defaults are not
 * legal advice. */
model SendingRules {
  /** The Frequency caps */
  frequencyCaps: FrequencyCap[];

  /** The Sending windows */
  sendingWindows: SendingWindow[];
}
