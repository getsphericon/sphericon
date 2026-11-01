---
status: accepted
---

# On-site popups and forms are built, as an effort of their own

Popups, banners and forms on the customer's site were added to the channels map
(getsphericon/sphericon#916). The research (`research/on-site-messages-and-forms`) found two roles:
a capture form for visitors (Klaviyo, OptinMonster, Sumo, Mautic) and an on-site message addressed
to a known person (Customer.io, Braze), which the big engines serve with one object; the web push
soft prompt is itself an on-site message. A first answer was to ship none and leave them to form
tools. The maintainer reversed it: on-site campaigns are one of the platform's main features, as
Drip Onsite is Drip's (pop-ups, slide-ins, sticky bars, sidebars, embedded forms; targeting by page,
Segment and cart; an SMS consent element; `docs/research/drip-com-feature-analysis.md`, section 6).

## Decisions

- **sphericon shows on-site popups, banners and forms** through its first-party tracker. They are
  a source of Identify, of Confirmations on every Channel (an SMS or WhatsApp consent element
  included) and of the web push permission prompt.
- **They are designed as an effort of their own,** not inside the channels map: their builder,
  display rules, targeting, frequency, accessibility and the way a browser's consent is made
  trustworthy (the tracker's collect key is public, and sphericon runs no SMS double opt-in of its
  own, ADR 0077) are a product the size of the channels themselves. The channels map only relies on
  them as a consent and Binding source.
- **The tracker also offers calls for the customer's own UI:** `subscribeToPush()`, called from a
  button the site draws (the user gesture browsers require), creates the web push Binding for the
  current Visitor (ADR 0075); and a Telegram start link for the current Visitor (ADR 0076).
