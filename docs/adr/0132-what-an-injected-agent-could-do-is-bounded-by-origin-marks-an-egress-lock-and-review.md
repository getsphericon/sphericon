---
status: accepted
---

# What an injected agent could do is bounded by origin marks, an egress lock and review

> **Amended by [[0136-a-wait-for-event-keeps-the-event-that-ended-it-for-later-steps]]:**
> an Event written through a Connection without `mcp:send` satisfies no Wait for event either.

> **Amended by [[0135-a-broadcast-series-makes-an-ordinary-broadcast-at-each-occurrence]]:**
> a feed's content is untrusted; a series' Publish shows its authors and the feed's hosts, which
> it approves for the issues that follow.

An agent operates a Workspace through MCP, a projection of the external API (ADR 0016). Much of what
it reads was written by strangers: a Contact's name, its Custom fields, the properties of its
Events, the text of a message it sent in. A string such as "ignore your instructions and..." in a
`first_name` reaches the agent's context the moment a tool returns it.

ADR 0016 already bounds what such an agent can do. It authors by default; a send needs `mcp:send`
and a box of its own on the consent screen; changing how the Workspace sends (Sending domains,
Tracking domains, Integrations) needs a third box; consent only narrows through it. Fields marked
`@untrusted` in the contract reach the agent wrapped as `{"untrusted_data": ...}`, and the server's
instructions say to treat them as data.

The locks guard the operations named send-class, but some writes granted by default take effect at
once, with no Publish or Schedule:

- `webhooks:write` points a Webhook endpoint at any URL, and every Event, with the Contacts it names,
  then flows there;
- `templates:write` edits a Template that Transactional sends render by reference at send time;
- `events:write` and `contacts:write` record Events and Tags that start live Automations and fire
  Rules, which send;
- a Broadcast's Test send, under `broadcasts:write`, mails the Broadcast's content to any address
  through the Workspace's Integration, outside Send eligibility.

And a person approving a send does not see that an agent drafted it, while the cheapest harm, a link
in a draft that a person then sends, is checked nowhere. The app runs no model of its own; this
decision is about agents that connect to it.

## Decision

### What is untrusted is decided by origin

- **A field is marked `@untrusted` when anyone other than a Member can write it**: a door of ingest
  (the tracker, collect, the API's event and upsert operations, Identify, an Inbound message, an
  On-site form submit, an Import), or a third party's answer. That is: a Contact's `subject_id`,
  names, core attributes written by Identify (city, country, locale, time zone, birthday, avatar) and
  Custom field values; a Custom field's key and name; a Tag's name; an Event's action and properties,
  which carry Inbound message text and order lines, and an Enrollment's `event` copy; a Captured
  message's content; the response excerpt an HTTP step or Webhook delivery stored.
- **Message content is wrapped too**: a Template's, a Broadcast's and a Step's content, and a Brand
  kit's text. It is written for recipients, never for the agent, and a Connection can write it, so a
  second agent must not read the first one's text as instructions.
- **The rule, not a list, is what a contract review applies**: a new field is marked when anyone but
  a Member can write it. Marks missing today (`subject_id`, the Identify core attributes, the `event`
  copy and the response excerpts in `contacts.tsp`, `enrollments.tsp`, `rules.tsp` and
  `webhooks.tsp`, and message content) are added.

### Nothing an agent writes sends without the send lock

- **A write made through a Connection that lacks `mcp:send` starts nothing that sends.** An Event it
  records, a Tag or a field it changes, and a Contact it creates carry a cause that marks them, as a
  Rule's Action marks what it publishes (ADR 0054), and such an Event enrolls no one, fires no Rule
  and records no `contact.score_reached` (ADR 0129). The send lock thus covers the sends a write
  would start, not only the operations named send-class. Webhook endpoints still receive them: that
  is data the Workspace already sends out.
- **Editing a Template that Transactional sends reference is send-class over MCP**: it changes mail
  that goes out at once, with no Publish (ADR 0099 names that Live use). A Template no Transactional
  send references is edited as before.
- **A Test send through a Connection goes only to the User it acts for**, as a Test send without a
  verified Sending domain does today; another address needs `mcp:send`. A test is for the author to
  see the mail, not a way to mail anyone outside Send eligibility.
- **Hand-made API tokens are unchanged**: they are the Workspace's own code, given scopes by an owner
  or admin, and keep `webhooks:write` and every effect of their writes.

### Egress is the third lock

- **`webhooks:write` becomes a Setup scope**: an OAuth client gets it only when it asks and the user
  ticks the setup box, and only from a User holding `webhooks:manage`. Creating a Webhook endpoint,
  changing its URL or the Events it receives is a standing way out for the Workspace's data, the same
  class as an Integration's credentials (ADR 0016, "Setup is a third lock"). Sending a test delivery
  and rotating an endpoint's secret open no new way out and stay as they are.
- **Existing Connections lose `webhooks:write`** until their User consents again with the box. An
  endpoint a Connection made stays, and the Audit log records who made it.
- **The other outbound URLs are behind the send lock already**: an HTTP step and a Rule's HTTP
  Action run only once a version is published, and Publish is send-class (ADR 0065).

### Who drafted it is shown where it is approved

- **Schedule and Publish show every author of a change since the last Schedule or Publish of that
  object**, not only the last one, so a person's one-word edit does not hide an agent's draft: each
  User, API token or Connection (with its client's host and the User it acts for). Schedule also
  shows who last changed the Segment, which decides who receives.
- **The authors are kept on the object.** The Audit log records content changes too, but it is an
  Enterprise feature absent from a plain install (ADR 0022), and the approver needs this in every
  install.

### Links are reviewed before a send, whoever wrote them

- **Schedule and Publish list every host the messages link to or load images from, and every URL of
  an HTTP step and a Rule's HTTP Action the version adds.** A host the Workspace has never used is
  marked new; a link whose host comes from a merge tag is marked as decided per recipient.
- **A host becomes known only by a send a person confirmed**: a Schedule or a Publish by a User, and
  the Workspace's Sending and Tracking domains. A Test send, a Transactional send and anything a
  Connection or token did make nothing known, so an agent cannot make its own host familiar.
- **It is a warning beside the Content checks (ADR 0130), never a block.** It applies to every
  author: a member pastes links too.
- **An image whose URL a non-Member wrote, such as a Contact's avatar, is shown only after a
  click**, so opening a Contact does not tell a stranger's server that a Member looked.

### Reading is the grantor's choice, said plainly

- **A Connection with `contacts:read` puts Contacts' personal data into the agent's context and its
  provider's.** The consent screen says so beside that scope. No limit beyond the rate limits (ADR 0025) is set: an agent trusted to read Contacts can read them.

## Consequences

- The scope vocabulary moves `webhooks:write` into `SetupScope`; a migration drops it from existing
  Connections.
- A write through a Connection without `mcp:send` carries a cause that Automations, Rules and Scores
  skip; a Template in Transactional use, and a Test send to another address, are send-class over MCP.
- Content objects keep their authors since their last Schedule or Publish; Schedule and Publish show
  them and list hosts with the new ones marked; the Workspace keeps its known hosts.
- The contract gains the missing `@untrusted` marks and wraps message content; the consent screen
  gains the warning on reading Contacts; images from non-Members load on a click.
- Not in this decision: Skills a Workspace writes, inline intents with a model inside the app (whose
  model, which data it sees), and marking objects as an agent's beyond their authors.

## Considered options

- **A taint flag on every value, carried into what an agent writes.** A model's output cannot be
  traced back to the strings that shaped it; the boundary that can be enforced is the tool result.
- **Blocking agent-drafted sends until a second person approves.** The send lock and the visible
  authors already make the approver a person; a second approver is a Workspace's own policy.
- **Blocking new link hosts.** A first newsletter links to new places; a warning where the person
  approves is enough.
- **Webhook endpoints token-only, as API tokens are.** A Workspace may want its agent to wire an
  integration; a box on the consent screen asks the person, as for Integrations.
- **Making `events:write` and `contacts:write` send-class.** An agent that only tidies Contacts or
  records history would need the send box; marking its writes as starting nothing keeps the scopes
  useful and closes the side door.
- **Limiting how many Contacts an agent may read.** A limit an agent can page around protects
  nothing; the choice is made when the scope is granted.
