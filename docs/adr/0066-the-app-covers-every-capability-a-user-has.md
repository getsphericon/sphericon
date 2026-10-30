---
status: accepted
---

# The app covers every capability a User has, through a form

sphericon is operated by agents first (ADR 0016), and the app had started to lean on that: an
Integration's Preset params were a JSON editor "for support" because agents are the primary
interface, an archived Integration or Sending domain could be archived but not restored, and an
Automation's HTTP step took its headers as free text and could not be dry-run as a Rule's action
can. A person who runs sphericon without an agent met each of these as a wall.

## Decisions

- **Every capability has a screen.** What a User can do through their surface (a Workspace member
  through `/site`, `/api` and MCP, a Staff member through the Staff console) they can do in the app.
  The unit is the capability, not the operation: a list that shows what a `get` returns covers the
  `get`.
- **Through a form, not a data editor.** A value whose structure is known (a JSON Schema, a list of
  headers, a set of fields) is edited in a form built for it, generated from the schema when the
  schema is dynamic. A raw JSON editor for such a value is a gap, not an alternative.
- **Free text stays text.** A value that is text or code by nature, with no structure known in
  advance (a Liquid template, an HTTP body, HTML source), may be a text or code editor.
- **Instance configuration is outside.** What the Self-hoster sets in files and environment
  variables (Preset files, the Instance's settings) is configuration of the installation, not a
  capability of a User; the app shows it read-only where the work needs it, as it shows a Preset's
  params schema.

## Considered options

Leaving the app as a support console behind the agents makes every new capability cheaper (a
contract and an MCP tool, no screen), and was rejected: the open app is also the product a
Self-hoster runs without an agent. Recording the rule only in `AGENTS.md` was rejected because a
reviewer would not check it; it is rule 16 of `docs/standards/review-rules.md`.

## Consequences

Each new capability costs its screen in the same change. A dynamic schema is rendered with a
ready-made library (rule 3) rather than narrowed to what a hand-written form can draw: Preset params
stay full JSON Schema 2020-12 and the app renders them with react-jsonschema-form's Mantine theme.
The gaps found when this was decided (Preset params, restoring an archived Integration or Sending
domain, the Automation HTTP step's headers and dry-run) are filed as issues.
