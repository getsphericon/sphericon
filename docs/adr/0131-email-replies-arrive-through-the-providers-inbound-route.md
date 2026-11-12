---
status: accepted
---

# Email replies arrive through the provider's inbound route

sphericon receives no email. Four things wait for it: an Automation that stops when the person
replies (left out of ADR 0124), an Event a Rule can react to when a Contact writes back, the
feedback loop reports Yahoo sends by mail (moved here by ADR 0130), and a `mailto:` in
`List-Unsubscribe` (deferred by ADR 0012, which ships only what it can act on).

SMS and Telegram already receive: a provider calls the Integration's Ingest hook, a Preset parses
the callback into an Inbound message, and the core reads what it means (ADR 0037, ADR 0076). ADR
0037 rejected a bare MTA, with VERP return paths and a mailbox parsing bounce reports: production
mail goes through a provider, and providers report bounces by callback. An email's `Reply-To` is
today only an Integration's default.

## Decision

### What it is for, and how mail arrives

- **Inbound email serves replies, Yahoo's feedback loop and `mailto:` unsubscribes.** Bounce reports
  by mail (DSN) stay out, as ADR 0037 decided, and `Return-Path` stays the provider's. Microsoft's
  JMRP is registered by IP, and a provider's IPs are shared, so the provider receives it.
- **Mail arrives through the provider's inbound route, as a Preset on the email Integration's Ingest
  hook**: an SES receipt rule through SNS, a Mailgun route, Postmark's inbound webhook, and
  `generic`; each verifies with a scheme ADR 0037 already has (SNS, Mailgun's body fields, a token
  or basic auth). sphericon runs no SMTP server. The email Inbound message carries what the core
  needs: the address it was sent to, `Message-ID`, `In-Reply-To`, the auto-reply headers, the
  provider's SPF, DKIM, DMARC and spam verdicts, the text, and the raw message for forwarding.
- **A message over the route's limit is kept by its headers.** The Preset engine parses at most
  1 MiB, and a provider may send less (SES puts at most 150 KB of content in an SNS notification).
  Such a reply is recorded from its headers and the text that arrived, and forwarded without the
  original, saying it was too large to carry.

### Reply handling, per Sending domain

- **Reply handling is an option of a Sending domain.** Turning it on needs a subdomain,
  `reply.<domain>`, whose MX points at the inbound route of one email Integration, checked like the
  DKIM record; and a **reply mailbox**, the team's address, confirmed by a link mailed to it and never
  on `reply.<domain>`.
- **Receiving names an Integration; sending still does not.** ADR 0010 keeps a Sending domain free of
  any one Integration, and every Integration still sends from it. The MX can point at one provider
  only, so the Sending domain names the Integration whose route receives its replies, and forwards go
  out through that Integration.
- **Each Outbound message gets a reply token**: a short random id stored on it, lower-case base32,
  so it fits an address's local part and survives a mail server that lower-cases one. The signed
  Message tokens of links are too long and case-sensitive for an address.
- **Then every email from the domain carries `Reply-To: r-<reply token>@reply.<domain>`**,
  Transactional included: a reply to a receipt is mail the team wants. A Notice, sent through the
  System sender, does not. It also carries an opaque message-token header, signed, for the feedback
  loop below.
- **A Broadcast's or an Automation's email also carries a `mailto:u-<reply token>@reply.<domain>`
  in `List-Unsubscribe`**, beside the https URI and in the same `h=` list. As ADR 0012 has it, a
  Transactional email and an opt-in confirmation mail carry no `List-Unsubscribe` at all.
- **With the option off, nothing changes**, and the Integration's default `Reply-To` applies.

### A reply reaches the team

- **Every reply with a valid token is forwarded to the reply mailbox** through the receiving
  Integration: `From` is `<the person's name> via <domain>` at an address of the Sending domain, so
  it passes DMARC; `Reply-To` is the person's address, so the team's answer goes straight to them; the
  original is attached. The name is inert text, never an address or a link. A forward carries no reply
  token and no `List-Unsubscribe`: it is not mail to a Contact.
- **This is not the arbitrary address ADR 0125 refused for Notify.** The reply mailbox is the
  Workspace's own, proven by a link it was mailed, and receives only what people wrote back to the
  Workspace. A forward is outside Send eligibility, and the route counts it as it counts every send.
- **A reply token is bounded**: at most ten replies a day are forwarded and recorded per token, so
  one recipient replaying a token cannot flood the team or the Event log.
- **The raw message is kept only until it is forwarded**, and an Erasure of the Contact purges a
  forward still queued.
- **Mail without a valid token, or that the provider marks as spam or a virus, is not forwarded** and
  creates nothing. Forwarding it would relay strangers' mail, spam above all, from the Workspace's
  own domain and spend its reputation.

### What is recorded

- **A reply from the Contact the token names is recorded as `email.replied`, an auto-reply as
  `email.auto_replied`**: analytical System events, the text cut at 1 KiB, as `<channel>.received`
  is (ADR 0076). Each names the Outbound message, so the Broadcast or Automation it answers. They are
  named for what they are: on email only mail carrying a reply token is recorded, so each answers a
  message, unlike a `<channel>.received`, which may begin a conversation.
- **`email.auto_replied` is a machine Event** (ADR 0053): it triggers no Rule and no Automation and
  ends nothing, so an Automation never answers an autoresponder.
- **An Erasure deletes both**, as it deletes `telegram.received`: they hold the person's own words.
- **Replies are not engagement tracking.** A reply is the person's own act and comes back whether or
  not the message was tracked (ADR 0120); both Events are two-valued, as the customer's Events are
  (ADR 0123).
- **An Automation version may exit on a reply.** With the option set, a human reply to an email of
  that Automation ends the Enrollment that sent the email, as `exited`, if it is still active. A late
  reply to an earlier run's email ends nothing (ADR 0127). It is an option, not an exit event: an exit
  event on `email.replied` would end the Enrollment on a reply to any message, and an exit event has
  no filter on properties.
- **An auto-reply is told only by its headers**: `Auto-Submitted` with any value but `no`, the one
  standard mark (RFC 3834); the common non-standard ones, `Precedence` of `auto_reply`, `bulk` or
  `junk` and `X-Autoreply` or `X-Autorespond`; and a null `Return-Path`, to which RFC 3834 forbids
  any answer. No guessing from the subject. An auto-reply ends no Enrollment and is forwarded,
  marked: "I no longer work here" is worth reading.
- **The reply token decides whose reply it is.** A reply from an address other than the Destination still
  counts, since the token was only in that person's mail; the event records that the sender differed
  and the DMARC verdict. A token whose Outbound message was erased names no one.

### Yahoo's feedback loop

- **The Workspace enrols its DKIM domain in Yahoo's feedback loop, giving `fbl@reply.<domain>`**:
  Yahoo's loop is keyed by the `d=` of the DKIM signature, and sphericon signs every email with the
  Workspace's domain (ADR 0010). The app shows how.
- **The core parses the ARF report, not a Preset**: ARF is a format, not a provider. A report
  carries the original's full headers, so the message-token header finds the Outbound message
  whatever Yahoo does with the recipient's address. A report is accepted only when its DKIM signature
  passes for a domain on the Instance's list of feedback loop operators; Yahoo signs as
  `arf.mail.yahoo.com`.
- **A report is a complaint like a provider's**: it records `email.complained` and a Suppression,
  deduplicated by Outbound message with a complaint the provider forwarded.

### `mailto:` unsubscribe

- **Mail to `u-<reply token>@reply.<domain>` records the opt-out the message's one-click `POST`
  records**: the reply token names the Outbound message, which names its source-scoped unsubscribe
  target (ADR 0012), never `everything`. It is not forwarded. The token is enough, as it is for the
  link.

## Consequences

- The email Presets gain inbound routes, and the email Inbound message its fields; the Sending
  domain gains reply handling, its subdomain check, a confirmed reply mailbox, and names the
  Integration whose route receives; an Outbound message gains a reply token.
- Outbound send writes the `Reply-To`, the `mailto:` and the message-token header when reply
  handling is on.
- `email.replied` and `email.auto_replied` join the System events; the Automation version gains
  exit on a reply.
- The feedback loop operators are an Instance list; the core gains an ARF parser.

## Considered options

- **An SMTP server of sphericon's own.** It needs port 25, which many clouds close, MX, TLS and
  spam filtering, a subsystem ADR 0037 already declined.
- **VERP `Return-Path` for bounce reports.** Providers report bounces by callback; rewriting their
  return path breaks that.
- **Keeping `Reply-To` as the team's address.** Replies would never pass through the app, so none
  could be seen.
- **Not forwarding.** Turning reply handling on would silently take the customers' replies away
  from the team.
- **Forwarding mail without a token.** It relays spam from the Workspace's domain.
- **Exit on a reply as an exit event.** It would exit on a reply to any message.
- **Detecting out-of-office by subject.** It guesses in every language; the headers say it.
- **Accepting any ARF.** Anyone could suppress a recipient by mailing a forged report.
- **The link's Message token in the address.** A signed token is longer than a local part allows
  and breaks when a server lower-cases it.
