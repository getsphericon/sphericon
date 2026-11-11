---
status: accepted
---

# An On-site form shows once per page view at most, and never again after a submit

Several live On-site forms (ADR 0085) may qualify on one page, and one may qualify on every page.
The vendors read for the map keep frequency in the browser, per form (Klaviyo, OptinMonster, Mautic,
Sumo), and Braze allows one triggered message per 30 seconds; none applies a cross-channel cap to
on-site messages. Decided in the frequency ticket
(getsphericon/sphericon#957).

## Decisions

- **After a submit a form never shows again in that browser.** After a close the author picks:
  never again, again after N days (7 by default), or at the next visit. A teaser follows its form.
- **One overlay per page view.** At most one popup or slide-in is on screen, and once it is closed
  or submitted no other overlay opens on that page view. A bar may show beside an overlay; embedded
  forms are not counted.
- **Form priority** (`high`, `normal`, `low`, the most recently published winning a tie) picks
  among forms that qualify at once, and picks the one form a Placement shows.
- **A capture form does not ask the known by default.** A new form's audience is anonymous Visitors,
  as Klaviyo's "don't show to existing profiles"; the author may widen it. A web push button hides
  when the browser has already allowed or blocked notifications, and a form with nothing else then
  does not show. sphericon infers nothing more from what a Contact already has.
- **The state lives in the browser,** beside the Visitor, and is not synced across devices. Safari
  wipes script storage after seven days without interaction, so a form
  closed long ago may return there; that is accepted. Before Tracker consent is granted the state
  is kept only as the visitor's own close, for the session (ADR 0093).
