# Language variants of a message

sphericon does not choose a message's language for the Contact. A Template, a Broadcast or an
Automation email has one Message content (a subject and a body). There is no default variant
plus a variant per language, and the send does not pick one by the Contact locale.

## Why this is out of scope

Which language a Contact gets is a choice of the Workspace that sends, not something the platform
decides for it. The tools for it are already there:

- **A Segment by locale.** The Contact locale is a core attribute (ADR 0072, `de`, `pt-BR`). A
  Workspace that writes in several languages builds one Segment per language (`locale` begins
  with `de`) and one Broadcast or Automation branch per Segment. A Custom field of its own (a
  `language` field, say) works the same way.
- **A Workspace per language** for a team that runs each language as its own audience.

Variants inside Message content would cost far more than those tools:

- a matching rule (`pt-BR` against `pt`, a fallback);
- a mandatory default;
- editor tabs per language;
- a stored variant on every Outbound message, and reports split by it;
- Transactional sends by Template reference choosing a variant too;
- and every channel's content (#928) carrying the same structure.

All of that is product logic that each sender would set up differently anyway.

## Prior requests

- #914: "Language variants of a message, chosen by the Contact locale"
