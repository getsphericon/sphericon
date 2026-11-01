---
status: accepted
---

# sphericon ships no on-site surfaces

Popups, banners and forms on the customer's site were added to the channels map
(getsphericon/sphericon#916) with the question whether they are needed. The research
(`research/on-site-messages-and-forms`) found two roles: an on-site message addressed to a known
person, a sending channel in Customer.io and Braze; and a capture form for anonymous visitors, the
product of Klaviyo, OptinMonster, Sumo and Mautic. The big engines do both with one object, and the
web push soft prompt is itself an on-site message. The map's standing rule is to integrate with
what an external service already does rather than repeat it.

## Decisions

- **No on-site Channel.** A popup or banner shown to a known Contact from a Broadcast or an
  Automation is not built in this effort: it needs its own display engine (targeting, frequency,
  layout, CSP, accessibility). It would be a Channel of its own if it comes.
- **No forms.** The customer's own forms or a form service feed sphericon: Identify and email
  through the tracker or the API as today. A Confirmation for SMS or WhatsApp comes only through
  the API from the customer's server, never from the browser: the tracker's collect key is public,
  so a browser path would let anyone sign a stranger's number up, and sphericon runs no SMS double
  opt-in of its own (ADR 0077). The customer's server checks the number (a code, an OTP) first.
- **The tracker offers calls, not UI.** `subscribeToPush()` asks for permission, subscribes and
  reports the Binding for the current Visitor (ADR 0075); the customer calls it from their own
  button, which gives the user gesture browsers require, and hosts the one-line service worker. The
  tracker also returns a Telegram start link for the current Visitor (ADR 0076). sphericon ships no
  prompt, bell, banner or widget.

## Consequences

- A customer without a form tool of their own has no sign-up surface from sphericon.
- The web push audience grows only as fast as the customer puts a subscribe button on the site.
