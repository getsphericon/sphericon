---
status: accepted
---

# Deliverability is observed through Feedback-IDs, Gmail's Postmaster data and content checks

A Workspace sees its deliverability through its own figures: the Complaint rate and the bounce rate
per Sending domain (ADR 0011), computed from the `email.complained`, `email.bounced` and
`email.delivered` Events its Integrations' Ingest hooks bring back. Three views it lacks are common
in the field:

- **Gmail's own view.** Gmail sends no per-message complaint to senders. It reports aggregates in
  Postmaster Tools: a domain's spam rate, its compliance with Gmail's sender requirements and, by
  the `Feedback-ID` header, a spam rate per identifier the sender chose. Since September 2025
  Postmaster Tools has no domain or IP reputation.
- **The feedback loops of Yahoo and Microsoft**, which send each complaint as an ARF report by
  mail.
- **A check of a message before it is sent.**

Gmail's header is `Feedback-ID: a:b:c:SenderId`: up to three identifiers and the sender's id,
counted per identifier and only above a volume. It must be signed by DKIM. Amazon SES writes the
header itself: the sender sets `a` and `b` through the tags `ses:feedback-id-a` and
`ses:feedback-id-b`, and SES fixes `c` and the SenderId. sphericon builds and signs every email
itself and hands SES the raw bytes (ADR 0010), so a header SES adds is not under the Workspace's
DKIM signature.

The Postmaster Tools API (v2) returns, per domain and day, metrics the caller names, among them
`SPAM_RATE` and `FEEDBACK_LOOP_SPAM_RATE` for a given identifier, aggregated by the From header's
domain by default; a separate call returns the domain's compliance status with Gmail's sender
requirements, `COMPLIANT` or `NEEDS_WORK`, per requirement. The read scope is
`postmaster.traffic.readonly`.

## Decision

### Scope

- **This decision covers the `Feedback-ID` header, Gmail Postmaster Tools and content checks.** The
  ARF feedback loops arrive by mail, so they belong to inbound mail, decided on its own; complaints
  that a provider forwards (SES does) already reach Suppression. Seed tests of inbox placement and
  Microsoft SNDS are left out: SNDS reports by IP, and a provider's IPs are shared.

### Feedback-ID

- **Every email a Workspace sends carries a `Feedback-ID`.** Its field `a` names the send's
  origin: an opaque id of the Broadcast or the Automation, or one of the fixed words
  `transactional`, `confirmation` (an opt-in confirmation mail), `preference` (a fresh link of the
  Preference page, ADR 0121) and `test` (a Test send). Its field `b` is an opaque id of the
  Workspace. A header every recipient can read carries no database id and no name. Each field has
  its own prefix, so no value repeats across fields, as Gmail asks. A Notice, sent by the Instance
  through the System sender, carries `notice` and no Workspace. It is not the Sending source of
  ADR 0012: a Broadcast's Sending source is `broadcasts`, and Gmail is asked for each Broadcast.
- **The Send route decides who writes it**, by its provider adapter. Through a provider that builds
  the header from inputs (SES's tags), sphericon passes `a` and `b` as those inputs, and the header is
  under the provider's DKIM signature, not the Workspace's. Through SMTP or a provider that leaves
  headers to the sender, sphericon writes the header itself, with a SenderId set by the Instance, and
  signs it in the DKIM `h=` list beside the `List-Unsubscribe` headers (ADR 0012). Through a provider
  that writes its own header with no input, sphericon writes none: two headers are malformed.
- **What reaches Gmail is proven per provider before it is relied on**: an acceptance send through
  the real provider shows one `Feedback-ID`, the fields sphericon meant, and which signature covers
  it. A message that cannot carry the header is still sent: it is an observation aid, unlike the
  unsubscribe headers whose absence fails a send (ADR 0012).

### Gmail Postmaster Tools

- **A Workspace may connect Gmail Postmaster Tools with a Google account, with the read scope
  only.** Once a day the app reads, for each of its Sending domains that the account may read in
  Postmaster Tools, what the API returns: the domain's spam rate, the spam rate of each `Feedback-ID`
  identifier Gmail reports, so per Broadcast and Automation where the volume is enough, and the
  domain's compliance status.
- **It is shown beside the Workspace's own figures, labelled as Gmail's and as the whole domain's.**
  Gmail counts every message with that From domain, the domain's other mail streams included (ADR
  0012), so it is not the Workspace's own rate. The figures are of the latest day Gmail reported,
  which is a day or more behind; a day Gmail skipped for low volume shows as no data, never as zero.
- **Gmail's own verdict is the item of Needs attention**: a compliance status of `NEEDS_WORK`, with
  the requirement Gmail names. The app sets no threshold of its own on Gmail's figures, and they
  never hold or stop a send.
- **The Instance operator configures the Google OAuth client.** Without one, the connection is not
  offered. The grant is kept encrypted, as an Integration's credentials are.
- **The connection is a configuration change**: it needs `sending_domains:manage`, it is audited
  (ADR 0022), and it names the Member who made it. It ends when that Member's Membership ends, and an
  expired or revoked grant is an item of Needs attention of its own, never figures quietly growing
  old.
- **What Gmail reported is stored by day**, per Sending domain and identifier, because it cannot be
  derived from the app's own Events. It is deleted by age as analytical Events are (ADR 0019), and
  with its Sending domain. A disconnected account keeps what was read and reads nothing more.

### Content checks

- **An email is checked against fixed rules, never scored.** The rules: rendered HTML over 102 KB,
  which Gmail clips; a message of images with no text; a link or an image not on `https`; a link
  through a URL shortener; an image with no `alt`; a merge tag that renders empty or fails for the
  preview Contact.
- **They are warnings, never holds.** They show in the editor, and at Schedule of a Broadcast and
  Publish of an Automation; a Template, which a Transactional send uses, shows them in its editor and
  when it is saved. A merge tag is checked with the sample merge data a Test send uses. They are not
  the checks that hold a Broadcast, which stay as they are; ADR 0012's missing-DMARC warning stands
  beside them in the same list. A spam score (SpamAssassin, a seed network's score) is not built in: it
  is a black box and a heavy dependency, against the stance of honest ratios over black-box scores.

## Consequences

- Outbound send writes or passes the `Feedback-ID` by route; a Preset declares how its provider
  takes it; the Instance gains a SenderId setting.
- The Workspace gains a Postmaster Tools connection, a daily read and its stored days; the Instance
  gains the Google OAuth client settings; Needs attention gains Gmail's `NEEDS_WORK` and a lost
  grant.
- The email editor, Schedule and Publish gain the content checks.
- The ARF feedback loops move to the inbound mail decision.

## Considered options

- **Reading Gmail's reputation.** Postmaster Tools no longer reports it.
- **A threshold of the app's own on Gmail's rate.** Gmail states its own verdict; a second line
  would disagree with it.
- **Holding sends on Gmail's spam rate.** It is a day late and covers Gmail alone; the Workspace's
  own Complaint rate is what automated decisions read.
- **A Feedback-ID only on marketing mail.** Transactional mail shares the domain's reputation, and a
  complaint about it is worth seeing apart.
- **A built-in spam score.** See above; the rules say what to fix, a score does not.
- **SNDS and seed tests.** SNDS reports shared IPs; seed tests need a network of mailboxes.
