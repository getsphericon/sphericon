---
status: accepted
---

# UI patterns follow Cloudscape, rendered in Mantine

The app's screens are lists, forms and detail pages over workspace resources, the shape of a cloud
console. Each screen's choices (how a delete is confirmed, when a form validates, what an empty
list says) were made one screen at a time, and an agent writing a new screen had no reference to
copy but the nearest existing one. We want one source of interface patterns, so that a new screen
and a review of it can both be checked against a written rule.

## Decisions

- **[Cloudscape](https://cloudscape.design/patterns) is the one source of interface patterns.** A
  pattern the app needs is taken from there, in the words of `docs/standards/ui.md`, and not from a
  second design system.
- **We take its rules, not its components.** The screens stay Mantine (`CODING_STANDARDS.md`: no
  custom CSS, theming through the theme). A rule is rendered with Mantine components, and the
  document says which one.
- **The rules live in `docs/standards/ui.md`.** Each has the Cloudscape page it comes from and,
  when the code already follows it, the component that does. A rule the code does not follow yet is
  marked as a target and has a ticket; the document does not describe code that does not exist.
- **A departure is written down.** A screen that does not follow a rule says so in its pull
  request, and a departure that repeats becomes a line in `ui.md`.
- **Patterns Cloudscape does not have are ours to settle, per ticket.** Builders (Segment
  conditions, Automations) and the Broadcast flow are outside it; what is settled for them is added
  to `ui.md` when it is.

## Considered options

- **Several systems, the best page of each.** Rejected: the rules differ where they overlap (Carbon
  makes an error a kind of empty state with its own copy, Cloudscape keeps it an alert or a toast),
  and an agent has to pick between them on every screen.
- **Carbon.** Read for empty states only: clear rules, but nothing on resource management, deletion
  or validation.
- **Polaris, Primer, Pajamas, Atlassian.** Not adopted. Polaris moved its pattern pages and they
  were not read; for the other three the search found a component index or ticket discussions, not
  pattern text we could cite. A source we cannot cite per rule is not one.
- **A third-party skill or plugin for design.** Rejected: those set typography, color and CSS,
  which the theme owns here, and an installed skill is read by one agent where a file in `docs/` is
  read by all.
- **Nothing: keep following the nearest screen.** Rejected: the screens already differ (every
  delete confirms the same way whatever it deletes; forms validate when submitted), and nothing
  says which is right.

## Consequences

- A change to a screen is reviewed against `ui.md` as the Standards axis of `code-review`
  (`docs/standards/review-rules.md`, rule 17).
- Cloudscape is a document we do not control. `ui.md` states each rule in our words with the link
  beside it, so a change on their side leaves our rules as they are until we choose to follow it.
- The AWS-specific parts of Cloudscape (service dashboards, density settings, card view, saved
  filter sets) are not adopted; `ui.md` lists only what the app uses.
