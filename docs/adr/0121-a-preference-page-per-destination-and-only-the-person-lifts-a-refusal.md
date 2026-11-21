---
status: accepted
build: none
terms: [Preference page, Public name]
---

# A Preference page per Destination, and only the person lifts a refusal

> **Amended below:** a return from `everything` on email from an older link goes through the
> confirmation email, and the `preference` message carries every other widening; a link's age is its
> message's; the page acts on email and Telegram and only shows a Channel stop on SMS and web push; a
> suppressed address gets no fresh link; the address is masked; an Automation or a Broadcast series
> is listed by its Public name; the fresh links share the confirmation emails' limit.

> **Amended by [[0190-a-person-may-cap-their-own-destination-at-one-message-a-week]]:** the page of an
> email or Telegram Destination offers a Personal frequency cap, set by anyone and cleared only by the
> person.

> **Amended by [[0179-a-web-push-binding-reaches-a-contact-only-by-proof]]:**
> a web push Destination's page shows its Channel stop and that the site's own button undoes it; no
> `preference` message is ever pushed.

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
  it; Suppression and a Channel stop do, and it shares ADR 0013's limit on confirmation emails per
  Destination (amended below: on email and Telegram only; from an older link a return from
  `everything` on email is the confirmation email itself).
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
- The fresh link is a new Outbound message kind, `preference` (ADR 0130), on email and Telegram
  (amended below), beside the confirmation email; it is a message the person asked for, in ADR
  0013's mode that only Suppression (and on Telegram a Channel stop) stops.
- The Unsubscribe module gains a lift of a source or of `everything` on email and of a source on
  Telegram, reachable only from the page's proven actions and from a confirmation link's
  `POST` (ADR 0013); no `/site`, `/api` or MCP operation widens.
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

## Amendment: the fresh link, a link's age, the Channels and what the page shows

Read against the code, the later ADRs and the open tickets before the spec, these points were left
open or had drifted.

- **A return from `everything` on email from an older link is a Confirmation request the person
  makes** (ADR 0013): the page asks for the confirmation email, and pressing Confirm on it lifts the
  `everything` opt-out and records the Confirmation, as ADR 0013 decides. The confirmation email is
  that proof already; a second message with the same meaning would only be a second way to get it
  wrong. Every other widening (a return to a source, a tracking grant, clearing a Personal frequency
  cap, ADR 0190) from an older link sends the `preference` message, on email or Telegram. A link of
  the last 7 days still acts on the page's own `POST`, a return from `everything` included.
- **The fresh links share the confirmation emails' limit**: at most 3 messages a person asked for
  (a confirmation email or a `preference` message) per Destination per Workspace in 24 hours, counted
  by the same throttle. Over it the page answers as if the message went, so the page says nothing
  about an address. The fresh link is sent only because the person asked for it on the page, never
  by the sender to ask whether they changed their mind.
- **A link's age is its message's.** The unsubscribe token names its Outbound message; the link is
  fresh when that message was sent no more than 7 days ago. A link that names no message (an opt-out
  recorded without one, an author's variable in a message whose row retention removed) counts as
  older. The tokens already in people's mailboxes keep working: they never expire, and none needs to
  change. The fresh link carries a token of its own kind, with its own audience, valid 7 days, naming
  the Workspace, the Channel, the Destination and, on Telegram, the Integration; it names no action,
  so the person chooses on the page it opens. Neither token binds an identity (ADR 0160).
- **Proof is a type.** Each writer that widens (the lift of an Unsubscribe, a tracking grant, clearing
  a Personal frequency cap) takes a proof value that only the page's and the confirmation link's
  verification construct, so a widening without proof does not compile, and a static check refuses
  the constructor anywhere else.
- **The page acts on email and Telegram.** On Telegram the `preference` message goes through the
  Integration of the link's message, or, when its row is gone, through the Destination's oldest
  Binding, the order a send uses. On SMS the page shows the Channel stop, since every SMS opt-out is
  Channel-wide (ADR 0077), and START as its undoing; on web push the Channel stop and the site's own
  button (ADR 0179). Neither sends a fresh link: a message that anyone holding a link could ask the
  Workspace to pay for would be a bombing vector on a paid Channel. A tracking grant there acts only
  from a link of the last 7 days. WhatsApp is placed on the page by that Channel's own decision.
- **A suppressed Destination** narrows as any other, and a widening from a link of the last 7 days is
  recorded, since consent and delivery are separate facts (ADR 0154). From an older link no message
  goes and the page says neutrally that the sender cannot deliver to the address now, without the
  cause. The page never lifts a Suppression; its owner does (ADR 0154).
- **The page shows the address masked** (`j***@example.com`; on Telegram the bot's name), so a
  forwarded message does not hand the whole address to whoever opens it. The masked address is a type
  of its own in the contract, so a raw address cannot be returned by the page's read.
- **The sources listed** are those of the Destination's Outbound messages still held and of its
  Unsubscribes; a source whose Automation no longer exists is not listed. Each Broadcast series sits
  under "all Broadcasts" (ADR 0135); while `broadcasts` stands opted out, the series show as covered
  by it and offer no return of their own.
- **An Automation and a Broadcast series are listed by their Public name**, a name written for the
  person, not the Workspace's own name for it. An Automation with a Send step cannot be published
  without one, nor a series created; the internal name is never filled in for it.
- **A message with no Sending source** (a Transactional email) renders the author's
  `{{ unsubscribe_url }}` as a link to the page with no source, not as nothing; it carries no
  `List-Unsubscribe`, and the one-click `POST` still refuses a token without a source.
- **A Personal frequency cap travels on a Merge as a refusal does** (ADR 0128, ADR 0190): it is a
  narrowing, recorded again on the Destination the survivor keeps, with the provenance `merge`.
- **A return records `<channel>.resubscribed`** with its source and how it was made (`page`, or
  `confirmation` for a confirmation link's lift), on email and Telegram, and is kept as long as the
  same Channel's `unsubscribed` Event, since a Destination's consent history reads both (ADR 0141).
  Its observation time is kept with the other consent observations (ADR 0154).
- **The first action from a marketing link is one press**, opting out of the message's source, so the
  opt-out is what the page offers first without a form or a pre-ticked box: each change is its own
  `POST`.
