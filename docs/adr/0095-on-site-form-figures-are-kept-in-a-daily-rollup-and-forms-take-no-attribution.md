---
status: accepted
---

# On-site form figures are kept in a daily rollup, and a form takes no attribution

An On-site form records `onsite_form.shown`, `.closed`, `.submitted` and `.clicked` on the Visitor
and its Contact, naming the form, its version and the device (ADR 0086). Events expire (ADR 0019),
so figures read only from them would forget a form's history; a Broadcast's figures come from its
Recipients and Outbound messages for the same reason. Revenue is credited to the Broadcast or
Automation of a Contact's last human click on any Channel (ADR 0083). Decided in the reporting
ticket (getsphericon/sphericon#959); the maintainer asked for best practice.

## Decisions

- **Figures, as the form tools report them:** views (`shown`), submits, submit rate (submits /
  views, as Klaviyo and OptinMonster count it), closes, clicks on a URL button, new Contacts the
  form created, Confirmations it brought per Channel (email double opt-in, SMS and WhatsApp START
  carrying its token) and web push and Telegram Bindings from its buttons. Per day in the Workspace
  time zone (ADR 0057), split by device. Unique visitors are not counted. A form shown on purpose
  (the URL parameter or a draft preview, ADR 0089) records nothing.
- **A daily rollup keeps them.** One row per form, version, day and device holds the counters,
  added to as the Events are recorded, and lives as long as the form; it does not expire with the
  Events.
- **A form takes no attribution.** It is not a send, so Attributed revenue stays the messages'
  (ADR 0083). A form has its own figure, **Captured revenue**: Orders of the Contacts it captured,
  placed inside the Attribution window after their Form submit. One Order may count for a message
  that was clicked and for the form that captured the person; the two answer different questions
  and are labelled apart.
- **Per version:** the form's page filters the figures by version, all versions by default, so an
  author sees what a change did.
- **Where:** the form's page shows totals, a daily chart and the device and version splits; the
  forms list shows views, submit rate and new Contacts. The Overview gains nothing (ADR 0073).
- **No step funnel yet.** A drop-off per step of a multistep form needs an Event per step reached,
  which ADR 0086 did not add.
