---
status: accepted
---

# A Preference page per Destination, and only the person lifts a refusal

> **Amended by [[0135-a-broadcast-series-makes-an-ordinary-broadcast-at-each-occurrence]]:**
> the page lists each Broadcast series a person receives by name, beside all Broadcasts, which
> covers them.

A person who receives mail from a Workspace can do one thing from it today: confirm an opt-out of
the source the link names, then follow a second link to leave everything (ADR 0012). ADR 0120 added
a second page for the Engagement tracking consent. Nothing lets the person see what they are opted
out of, and nothing lets them come back: the glossary calls an Unsubscribe toggleable, but no path
lifts an email opt-out, neither for the person nor for a member. Commercial senders give one page
for all of it, reached from the unsubscribe link.

Coming back is the one hard part. An opt-out narrows reach and is safe whoever makes it; lifting a
refusal undoes what the person said, so it must be the person's own act, on proof that the address
is theirs. Consent a customer collected elsewhere (an imported Confirmation, ADR 0077; an imported
tracking grant, ADR 0120) is different: it is recorded where no refusal stands, and it lifts none.

## Decision

- **One Preference page per (Workspace, Channel, Destination)** replaces the unsubscribe
  confirmation page and the `/unsubscribed` page of ADR 0012 and the tracking page of ADR 0120. The
  footer of a Broadcast and an Automation email carries one link to it ("unsubscribe or change your
  preferences"). It is reached by the signed link that already names the Channel, the Destination,
  the Workspace and, from a marketing message, its Sending source.
- **The page shows that Destination only.** A link proves that someone reads that address, nothing
  about the Contact's other addresses.
- **It lists the Sending sources the Destination received from or is opted out of**, `broadcasts`
  and `everything`, each by its display name, with the Engagement tracking consent. Listing every
  Automation would publish the Workspace's internal names.
- **From a marketing message, opting out of its source is the first action**, as the footer link
  promised. A link without a source (a Transactional message, the author's variable) opens the same
  page with nothing first.
- **Narrowing takes effect at once, from any link however old**: opting out of a source or of
  everything, refusing tracking. The `List-Unsubscribe` one-click `POST` of the same URL still opts
  out of the message's source with no page (ADR 0012).
- **Widening on the page is the person's act on fresh proof of the address**: returning to a
  source, returning from `everything`, granting tracking. A link from a message sent no more than
  7 days ago (the lifetime of a confirmation link, ADR 0013) acts on the page's `POST`. An older
  link, or one from no message, sends a fresh link to the Destination on the same Channel (an email,
  or a message of the Integration the link's message came through) and acts on the `POST` of the
  page that fresh link opens; its `GET` changes nothing, as ADR 0013 requires of a confirmation
  link. The fresh link is a message the person asked for on the page, so `everything` does not stop
  it; Suppression and a Channel stop do, and it is deduplicated per Destination, as ADR 0013's
  confirmation mail is.
- **A return** deletes the Unsubscribe row of its source and records `<channel>.resubscribed` with
  that source, an evidentiary Event (ADR 0019). A return from `everything` on email also records a
  Confirmation with provenance `double_opt_in`, whatever the opt-in policy, since the proof is the
  same: a person acting on a fresh link delivered to the address. Its `marketing.confirmed` starts
  what a Workspace triggers on it, a welcome Automation included. A return enrolls nobody again in
  what the opt-out ended, and a Broadcast's unsubscribe count does not change: attribution is its
  own fact (ADR 0115).
- **Only the person lifts a refusal.** Members, the API and MCP record opt-outs and refusals, and
  the consent a customer collected where no refusal stands; none of them lifts an opt-out or a
  `denied`. The person never loses the way back: any message they kept leads to the page. ADR
  0016's rule for agents becomes the rule for everyone, and ADR 0120's `manual` grant no longer
  lifts a `denied`.
- **On SMS and Telegram the page shows a Channel stop and how to undo it**, in the Channel: START to
  the number, `/start` to the bot. Twilio refuses to send to a number that texted STOP until it
  texts START, and a blocked bot cannot write until the person unblocks it, so a return made on a
  page could not be honoured. A Telegram opt-out of one source, which only a link makes, is
  returned on the page like an email one.
- **When no Contact holds the Destination**, the page offers narrowing only: a return would keep the
  address of someone erased, for nothing they asked.
- **Each change is its own `POST`**; `GET` changes nothing, so a link scanner changes nothing, and a
  closed tab loses no choice already made.
- **The page is in the Instance's language and in the Workspace's Brand kit**, read live: it is the
  sender's page, not sphericon's. Pages in a Contact's locale wait for the app's second language,
  for every public page at once.

## Consequences

- The unsubscribe token routes to the page; the page's actions are their own endpoints, so the
  one-click `POST` keeps a single meaning. A page link may carry no source (a Transactional message,
  the author's variable): the token's decoding accepts that for the page, and the one-click `POST`
  still refuses it. The `/unsubscribed` route and its `all` link go.
- The fresh link is a new System message on every Channel the page serves, beside the confirmation
  mail, in ADR 0013's third eligibility mode with `everything` waived.
- The Unsubscribe module gains a lift of a source or of `everything` on email and of a source on
  Telegram, reachable only from the page's proven actions; no `/site`, `/api` or MCP operation
  widens.
- The email footer loses ADR 0120's separate tracking link.

## Considered options

- **A member lifts an opt-out on the person's request.** It lets a Workspace undo a refusal with no trace of the
  person's will, and the person needs no one: their old mail leads to the page.
- **Every link acts at once, however old.** A forwarded or leaked message would let anyone resubscribe
  the address and record it as the person's consent.
- **Every widening sends a fresh link.** It sends mail to someone who just acted on mail they
  received this week, for no extra proof.
- **A page of all the Contact's addresses.** The link proves one of them.
- **A return to everything on SMS or Telegram from the page.** The provider and the platform would
  refuse the sends the page promised.
