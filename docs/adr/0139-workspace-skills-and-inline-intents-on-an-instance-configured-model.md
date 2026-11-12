---
status: accepted
---

# Workspace Skills, and inline intents on a model the Instance configures

An agent operates a Workspace through MCP (ADR 0016) and is bounded by origin marks, an egress lock
and review (ADR 0132). The app ships playbooks for it as MCP prompts (welcome, win-back,
list-hygiene, set-up-sending-domain), Markdown files in the binary, and runs no model of its own.
Three things were left open: playbooks a Workspace writes about itself, help inside the app's own
editors, and telling an agent's work apart in the lists. A built-in chat as the main surface stays
rejected: what an assistant makes is a governed object, and its home is that object's editor.

## Decision

### Workspace Skills

- **A Skill is a named Markdown document a Workspace writes**, such as its brand voice or how it runs
  a win-back, with the shape of a shipped prompt: a name, a description and a body, no arguments.
  Every MCP client of the Workspace, a Connection or an API token, lists the Workspace's Skills as MCP
  prompts named `skill_<name>`, beside the shipped ones, so no Skill ever takes a shipped prompt's
  name, now or after a release adds one.
- **Skills are edited only in the app, by a Member holding a new Permission, `skills:manage`** (owner
  and admin by default), and each change is audited (ADR 0022). Neither the external API nor MCP can
  write one: a Skill is an instruction to every future agent, so an agent that read an injected string
  must not be able to plant one.
- **A Skill is an instruction, so it is not wrapped as data** the way message content is (ADR 0132):
  a Member wrote it for agents to follow. It has no versions, since it sends nothing; the editor warns
  when it names a tool the current MCP surface lacks, as a test does for the shipped prompts.

### Inline intents run on the Instance's model

- **The Instance operator configures the model**: a provider, its endpoint, key and model, in the
  Instance's settings. Without them there are no inline intents, and nothing else depends on them. A
  Workspace has no key of its own. The endpoint is the operator's own configuration, reached
  directly rather than through the SSRF-hardened client, so a self-hoster's model on a private
  address works.
- **The owner turns them on for the Workspace, off by default, under a new owner-only Permission,
  `intents:manage`**: doing so accepts the operator's model provider as a processor of the Workspace's
  definitions, a decision for the person who answers for the Workspace. The switch names the provider
  and the model being accepted, and the Workspace keeps who accepted them and when, in every install;
  the change is also audited where the Audit log exists. When the operator changes the provider or
  the model, the switch turns off until an owner accepts the new ones.
- **The model gets definitions and figures, never a Contact.** For a Segment it gets the schema (core
  attributes, Custom fields, Tags, Event actions, Scores), not the Conditions already saved, which may
  hold an address a Member typed. For a draft it gets the brief the Member typed, the draft in the
  editor, the Brand kit and the Workspace's Skills. For an explanation it gets a report's figures and
  labels (Step names, the links as authored), not an HTTP step's stored response or any list of
  Contacts. Never a Contact's row, a Recipient or a message sent to someone. What ingest wrote among
  the definitions, such as a Custom field's key or a Tag's name, reaches it wrapped as data (ADR
  0132).
- **The app keeps no prompt and no answer**: it counts them. What remains is what a Member saves.
- **The first intents each fill a draft in their own editor**: a description becomes a Condition in
  the Segment builder, checked as any Condition is; a brief becomes Message content in the composer;
  "explain" writes a reading of a report's figures beside it, kept nowhere. Nothing is written until
  the Member saves, and what is saved records its author as the assistant for that Member, shown at
  Schedule and Publish with every other author (ADR 0132).
- **What the model writes loads nothing before a person approves it.** An explanation is plain text,
  never rendered as markup, links or images. An assistant-filled draft's preview shows an image whose
  host is not a Known host only after a click, as ADR 0132 holds a non-Member's image, and its links
  face the review of hosts at Schedule and Publish. An injected definition cannot make the model's
  output send the Workspace's figures to a stranger's server as it renders.
- **Use is counted and capped.** Tokens used by intents are a metric of the Usage snapshot (ADR 0009),
  summed per Workspace over the Usage period, and the Workspace's settings show its use. The Instance
  sets a daily cap per Workspace, counted by UTC day as the Usage period is, past which the intents
  wait for the next day.

### An agent's work in the lists

- **The lists of Broadcasts, Automations, Segments and Templates filter by the kinds of author an
  object has had**: a User, an API token, a Connection or the assistant, counted since its last
  Schedule or Publish where it has one (ADR 0132), and since it was created where it has none (a
  Segment, a Template). Filtering by the last author alone would hide an agent's draft after a
  one-word edit by a person.

## Consequences

- The Workspace gains Skills, served as MCP prompts named `skill_<name>`; the Permission catalogue
  gains `skills:manage` and `intents:manage`.
- The Instance gains the model's settings and a daily cap; the Workspace a switch with its recorded
  acceptance; the Usage snapshot a metric of intent tokens; three editors gain their intent.
- Authors gain the kind "assistant"; the lists gain the filter by kinds of author.

## Considered options

- **Skills editable through MCP.** An injected agent would write the instructions every later agent
  follows.
- **A Workspace's own model key.** It splits where a Workspace's definitions go per Workspace and
  leaves the operator unable to say which processors an Instance uses.
- **Sending Contacts to the model**, for richer drafts. Definitions and figures are enough for a
  Condition, a draft and an explanation, and keep personal data out of a third party.
- **A built-in chat.** Rejected again: a conversation is a poor home for governed objects.
