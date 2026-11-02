---
status: accepted
---

# An On-site form is a versioned definition, and its Publish is send-class

ADR 0084 decided that sphericon shows popups, banners and forms on the customer's site through its
tracker, designed by a map of their own (getsphericon/sphericon#948). Its first ticket
(getsphericon/sphericon#952) settled what the thing is. The research for that map found that the
vendors converge on one content model (`research/on-site-builders`): a short run of screens, an
optional success screen, a teaser that belongs to the same message, one design with per-device
overrides. ActiveCampaign names the two roles apart: **Forms** (inline, floating bar, floating box,
modal) capture visitors, and **Site messages** are shown to known contacts by an automation action.
The map keeps only the first role.

## Decisions

- **It is an On-site form, not a message.** Every "message" in the glossary is something sent: an
  Outbound message, its Message content, a Captured message. An On-site form is not a Channel, never
  makes an Outbound message, and what it shows is not Message content. A form with no field (a notice
  bar) is still an On-site form, as a Klaviyo sign-up form can be a banner. "Site message" stays the
  name of the role this map does not build.
- **Four Formats, fixed at creation:** `popup`, `slide-in`, `bar` and `embedded`. A teaser is part
  of a popup or a slide-in, not a Format. A form changes Format only through **Duplicate**, which
  makes a new inactive form and leaves out what the new Format cannot hold. The display rules, the
  teaser and the Placement depend on the Format, so changing it in place would leave a draft whose
  parts no longer fit.
- **An embedded form fills a Placement:** a name the site puts on an element it reserves, not a
  record. The site names a place, not a form, so a snippet installed on a customer's site (ADR 0036)
  never changes when another form takes the place.
- **It is a Versioned definition, like an Automation and a Rule (ADR 0065).** Its version holds
  everything that decides who sees what: the content (steps, success step, teaser, per-device
  overrides, styles copied at authoring as ADR 0003 copies a Template), the display rules, the
  audience, the frequency, the Placement and the schedule. The name, the Format, the status and
  archiving stay on the form. The Events of a form name the version that was shown.
- **Publish is send-class,** under a scope of its own (`onsite_forms:publish`, and `mcp:send` over
  MCP), though nothing is sent. A live form puts content in front of the public on the customer's
  site and records Contacts and their consent; a half-edited live popup is seen by every visitor
  at once. ADR 0065's rule, that a token without a send scope cannot change what an armed surface
  does, covers it for the same reason.

## Considered options

- **A Format changeable in the draft.** Rejected: a teaser, triggers and a Placement fit one Format
  and not another, so every change would need rules for what to drop; Duplicate does it once, into
  a form that has not gone live.
- **Edits that go live at once, with a status only.** Simpler, but a live form shows each save to
  every visitor, and an API token with a write scope could change a live consent form.
- **The site names a form by its id.** Rejected: replacing the form in a place would mean editing
  the customer's site.
