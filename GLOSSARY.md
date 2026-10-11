# sphericon

Marketing automation platform. The domain is workspace-scoped (multi-tenant): every
contact, event, segment, and tracking entity belongs to exactly one Workspace.

## Language

### Core

**Workspace**:
The tenant boundary. Every domain entity belongs to exactly one Workspace, and all
queries are scoped by it. Owns its own secret keys (collect key for browser tracking,
ingest key for inbound provider webhooks).
_Avoid_: Account, tenant, organization, project

**Contact**:
The single record for a person in a workspace — whether fully identified or still anonymous.
Anchored by a stable internal id, with email, phone, and subject_id (the customer's own user
id) as alias keys, each unique per workspace and any of which may be absent. Carries
Custom fields (typed, named attributes beyond the core fields) and owns the Visitors
(devices) seen as this person.
The thing Events attach to and the thing a campaign is sent to. Whether a message may be sent
is _derived_ (see Send-eligibility), never a stored status — so anonymous, un-consented
Contacts are harmless. There is no separate tracking "profile": this entity is both the
behavioral identity and the marketing audience member.
_Avoid_: Profile, tracking profile, lead, subscriber, user, recipient

**Custom field**:
A workspace-defined, typed, named attribute on the Contact, beyond the core fields
(email / phone / subject_id / name). The one attribute concept — there is no separate,
schemaless "trait". An unknown key arriving from Identify or an Event payload is
_auto-created_ as a Custom field with an inferred type (declared-by-use), but it is a
first-class, typed, renameable definition from the first sight, not an anonymous bag of
keys — so the Segment builder always has a real, governed field list to offer. Core fields
and Custom fields together are a Contact's attributes.
_Avoid_: Trait, property, attribute (unqualified), metadata

**Tag**:
A named, workspace-scoped label applied to Contacts — by hand, by import, by an Automation
step, or through the API. Carries presence only (a Contact has the Tag or does not), no type
and no value; that is what separates it from a Custom field. Auto-created on first use, like
a Custom field. A Tag is an attribute, never a target set: "send to everyone tagged X" is
always a Segment with a has-Tag rule, so Tag does not reintroduce the rejected List model.
_Avoid_: Label, list, group, category

**Visitor**:
An anonymous device/browser identity — a `visitor_id` cookie, unique per workspace. Resolves
to a Contact once Identify establishes who it is; before that it may belong to no Contact. One
Contact owns many Visitors (the same person across devices).
_Avoid_: Tracking visitor, session, device, anonymous user

**Identify** (identity resolution):
The act of binding a Visitor to a Contact and asserting that Contact's alias keys
(subject_id / email / phone). It stitches the Visitor's earlier anonymous Events onto the
Contact, so pre-identify behavior becomes visible to segmentation.
_Avoid_: Alias, merge, reconcile

**Event**:
An immutable, append-only record that something happened (an `action`, optional properties,
at a time). Attached to a Contact by **stable identity resolved at ingest** — not by email —
and carries denormalized identity snapshot fields for debugging. Never FK-constrained to the
mutable Contact. The raw material behind behavioral segmentation. sphericon's own delivery and
engagement facts (`email.sent`, `email.opened`, `email.clicked`) are Events too — reserved,
system-generated actions — so engagement is segmentable through the very same machinery as
customer-tracked actions, with no parallel event stream.
_Avoid_: Activity, log entry, signal

**Erasure**:
Removing a Contact's personal data at the Workspace's request (GDPR Art. 17). It is what deleting
a Contact _is_ — there is no softer delete that keeps personal data. The Contact, its Visitors,
Custom field values, Tags and Confirmation are removed, as are its customer-tracked Events; the
reserved `email.*` Events and delivery records stay as anonymous rows, so deliverability and
metering counts do not change. What survives is the minimum that keeps the refusal honored: the
Unsubscribe and Suppression rows for its Destinations. It leaves a PII-free `contact.erased`
Event, stored without any Contact reference (its webhook delivery carries the customer's own
subject_id so downstream copies can be erased). A person who later reappears (Identify, import) is a new Contact and
is not blocked; surviving opt-outs still apply to them. The Workspace is the controller and sphericon
the processor, so Erasure is an operator action, never a data-subject self-service form.
_Avoid_: Anonymization (that is only what happens to the delivery rows), forget, purge (purge is
bulk retention policy, a separate concern)

**Data export** (subject access):
A machine-readable bundle of everything held about one Contact (GDPR Art. 15/20): the Contact,
its attributes, Tags, Visitors, Events, opt-outs and delivery metadata — not rendered message
bodies.
_Avoid_: Backup, dump, contact export (that is the bulk list export)

**Retention**:
How long an Event is kept before it is deleted. Events split by purpose: **evidentiary** Events
(consent proof such as `marketing.confirmed`, permanent bounces and complaints, unsubscribes)
are the audit and dispute record and are never deleted by age; **analytical** Events (opens, clicks,
deliveries, customer-tracked actions) are deleted once older than the instance's retention window,
so segment conditions cannot look back past it.
_Avoid_: TTL, expiry, purge, archive

**Segment**:
A named, reusable definition of "which contacts match these conditions" — the one
targeting primitive. Stores a rule query (combinator + nested rules over contact fields,
custom fields, tags, and events), evaluated live: membership is never materialized and shifts as
data changes. There is no other segment kind; a segment is always a rule.
_Avoid_: List, audience, group, filter, snapshot

### Send-eligibility

Whether a message on channel C from sending source S may reach destination D is decided in
layers — never by a single flag on the Contact: (1) (C, D) in Suppression → never; (2) (C, D)
unsubscribed from _everything_ → never; (3) (C, D) unsubscribed from S → never; (4) _if the
workspace requires confirmed opt-in_ and (C, D) has no Confirmation → never (the one positive
layer, evaluated last so the negatives always dominate); (5) otherwise, send. Transactional
messages skip layers 2–4 (they still respect Suppression's hard bounces). Consent is
per-channel: an email opt-out never silences SMS.

**Destination**:
The channel-specific address a message is sent to — an email address on the email channel, a
phone number on SMS, a messenger id on a bot channel. Suppression and Unsubscribe are keyed by
(channel, destination); a Contact may have several (its email, its phone).
_Avoid_: Address (unqualified), recipient, endpoint

**Suppression**:
The workspace's authoritative, (channel, destination)-keyed do-not-send registry for hard,
global facts: hard bounce, spam complaint, and manual bans. Destination-centric (covers
destinations with no contact) and a compliance ratchet (bounce/complaint do not auto-clear).
The send path's global hard floor, checked on every send.
_Avoid_: Blacklist, denylist, do-not-mail

**Sending source** (the unsubscribe scope):
The unit a Contact unsubscribes from — and it is automatic, never hand-authored: the source
_is_ the sender. Each Automation is its own scope (unsubscribing from one drip leaves the
others untouched); all Broadcasts share one scope ("broadcasts"); and a reserved _everything_
scope is the deliberate "leave entirely" opt-out (kept separate so a per-source unsubscribe
never silently loses the whole contact). A source only ever _subtracts_ opt-outs from a send
— it is never a target set ("send to everyone subscribed to source X" is the rejected List
model leaking back; targeting is always a Segment). In the email, the unsubscribe link is
labelled with the source's display name ("unsubscribe from this mailing list") — that
"mailing list" wording is user-facing copy only, not a reintroduction of the List concept.
_Avoid_: Topic, list, mailing list, subscription group

**Unsubscribe** (the record):
A per-(workspace, channel, destination, Sending source) opt-out from marketing — keyed by
destination, not by Contact, so it survives contact deletion and re-import (the GDPR-erasure
case: the opt-out is the minimum data retained to honor the refusal). Toggleable: a contact can
resubscribe.
Absence means subscribed — there is no positive subscription row and no "subscribed" flag on
the Contact. The default in-email link unsubscribes from the _sending source_ only;
"unsubscribe from everything" is a distinct, deliberate action. A bounce or complaint is not
an Unsubscribe; those are Suppression entries.
The opt-out is offered two ways with **identical scope**: the visible footer link and an RFC 8058
**one-click** `List-Unsubscribe` header (Broadcast + Automation only, never Transactional — see
ADR 0012), both carrying the same source-scoped token; a mailbox provider's one-click button never
reaches the `everything` scope. The endpoint is **method-split for safety**: `GET` renders a
confirmation page and changes nothing, `POST` performs the opt-out — so link scanners and security
proxies that GET every email link cannot silently unsubscribe a live subscriber.
_Avoid_: Opt-out flag, status

The **scope** of an unsubscribe (what the contact is opted out of) is distinct from its
**attribution** (which sent message provoked it). A broadcast unsubscribe has scope
"broadcasts" (shared) yet is attributed to the one broadcast whose email triggered it — two
separate facts, never one field.

**Confirmation** (confirmed opt-in):
The **positive** mirror of an Unsubscribe: a per-(workspace, channel, destination) consent fact,
recorded as a derived destination-keyed row (the fast eligibility gate) written alongside an
immutable `marketing.confirmed` **Event** (the source of truth — GDPR Art. 7 "demonstrate
consent" proof, and a bus signal that can trigger a welcome Automation). It exists so double
opt-in can be modeled **without** reintroducing a subscription status: the truth is an event, not
a mutable flag. It matters only when the workspace's **require-confirmed-opt-in** policy is on
(default off = today's single-opt-in, unaffected); then Send-eligibility adds the one positive
layer (see Send-eligibility). Carries **provenance** — `double_opt_in` (clicked a confirmation
link), `grandfathered` (existing contacts at policy-enablement), or `imported` (importer-asserted
prior basis); the strongest provenance wins on dedup. Confirmed via a signed link whose endpoint
is method-split (`GET` = page, `POST` = confirm) — a scanner GET must never fabricate consent. An
`everything` unsubscribe invalidates the gate row (the event log keeps the history), so returning
after a full opt-out requires re-confirmation. Unlike an Unsubscribe it is **not** compliance-
required to survive erasure — losing it merely forces re-confirmation, which fails safe.
_Avoid_: Subscribed flag, subscription status, opt-in flag, double opt-in status

### Sending

**Broadcast**:
A one-off email send to an audience, authored and sent once (draft → scheduled → sending →
sent / failed). Its audience is a Segment (or all Contacts) resolved and **frozen into
Recipients at send time**, then filtered per-recipient by Send-eligibility. Sends under the
shared "broadcasts" unsubscribe scope. There is no "active contacts" audience — eligibility is
derived, not a contact status.
_Avoid_: Campaign, newsletter blast, mailshot, email blast

**Broadcast recipient**:
The per-(Broadcast, Contact) record — the frozen audience snapshot taken at send
time, pointing at its Outbound message for delivery state (sent / failed), and the home of a denormalized
**rollup** of engagement (opened / clicked) derived from the underlying `email.opened` /
`email.clicked` Events, for per-broadcast stats. The Events are the source of truth; the
rollup is a convenience view. Contrast Segment, which is a live query and never materialized.
_Avoid_: Send log, delivery row

**Outbound send**:
The one act of sending a single email on any surface (Broadcast, Automation step,
Transactional): Send-eligibility, the Workspace freeze check, the Sending-domain gate,
rendering, the unsubscribe footer and headers, DKIM signing, the provider call, and the record of
the result. Every surface goes through it; none re-implements any part.
_Avoid_: Delivery pipeline, send path (unqualified), mailer

**Outbound message**:
The durable record of one Outbound send — which surface and which of its records it belongs to,
the Sending source (none for Transactional), the Sending domain, the provider's message id, and
the outcome. Claimed before the provider is called, under a per-logical-send idempotency key, so a
retry never sends twice. Holds provenance and outcome, never rendered content. A Broadcast
recipient points at its Outbound message; it does not replace it.
_Avoid_: Send log, delivery row, email record

**Outcome** (of an Outbound send):
Either **per-recipient** and final for that destination — _Sent_, _Skipped_ (Suppression,
Unsubscribe, or missing Confirmation), or a permanent failure of that one message — or
**per-source** and reversible: a **Hold** (Workspace suspension, Billing hold, an unverified
Sending domain, or no Integration) or a **Deferral** (the Integration's Send rate limit is spent),
after which the same messages may still be sent.
_Avoid_: Status, result (unqualified)

**Send rate limit**:
The ceiling an Integration puts on how fast mail may leave it: a maximum messages per second and a
maximum messages per rolling 24 hours, mirroring what the external provider account allows.
The provider reports its own ceiling where it can (SES) and an operator may set one; the lower of
the two applies (the operator's on a tie), and with neither the Integration is unlimited. Applies
to Broadcast and Automation sends; Transactional sends are never delayed or deferred by it but
still count against the 24-hour total.
_Avoid_: Throttle (as a noun for the limit), quota (quota is enforcement of billing, not this)

**Deferral**:
The reversible, per-source Outcome of an Outbound send that found its Integration's Send rate
limit spent: the message is not consumed and is retried when capacity returns. Unlike a _Hold_,
nothing is wrong with the source — it is simply busy — so it carries no hold reason and a
Broadcast stays _sending_ with a progress estimate instead of a blocked state.
_Avoid_: Throttled, rate-limited (as an outcome), delay

**Hold**:
The reversible, per-source Outcome that stops sending without consuming recipients: a Broadcast
pauses with its remaining recipients pending, Automation Enrollments wait, a Transactional request
is refused. Distinct from _Skipped_, which is final for one destination.
_Avoid_: Block, failure, ban

**Sent** (vs delivered):
"Sent" means a message was _accepted by the email provider_ — not that it reached the inbox.
True delivery failures surface afterward as bounces / complaints, which flow into Suppression.
A broadcast's delivery rate is accepted-by-provider ÷ targeted, not inbox delivery.
_Avoid_: Delivered (for the accepted-by-provider sense)

**Transactional send**:
A single-recipient email triggered by the customer's own application through the `/api`
surface (e.g. password reset, receipt, OTP) — the third send surface alongside Broadcast and
Automation, and what makes sphericon one service for marketing _and_ transactional. It carries no
Sending source and is **never** authored as a campaign: the app supplies a Destination, a
referenced Template id, and per-call variables, and the content is rendered at send time. It
skips Unsubscribe (Send-eligibility layers 2–3 — you cannot opt out of your own password
reset) but **still respects Suppression** (a hard-bounced or complained address is never
sent to, transactional or not). Unlike marketing, it binds its Template by _reference_, not
by copy (see Template).
_Avoid_: Notification, system email, trigger (that is the Automation term), API send

**Automation**:
A workspace-scoped, event-triggered sequence: when its Trigger fires for a Contact, the
Contact is enrolled and walks the Automation's ordered steps (send email, wait, …). Each
Automation is its own unsubscribe Sending source. Activating starts enrolling; deactivating
stops _new_ enrollments but lets in-flight Enrollments finish (deactivate ≠ halt).
_Avoid_: Workflow, flow, journey, drip, campaign

**Trigger**:
The single Event action that enrolls a Contact into an Automation (e.g. "contact.created", a
custom event). Not a separate entity — one Trigger per Automation. Matched by the Event's
resolved identity, not by email.
_Avoid_: Hook, rule, entry condition

**Enrollment**:
One Contact's membership-and-progress in one Automation — enrolled at most once ever
(re-enrollment is deliberately out of scope). Holds the current step and a terminal state:
`completed`, `failed`, or **`exited`** (left early). An unsubscribe from the Automation has two
effects from one action — it records the durable opt-out (scope `automation:<id>`) _and_ moves
the active Enrollment to `exited`; suppression and hard bounces likewise exit the Enrollment.
A run never silently keeps walking steps while skipping every email.
_Avoid_: Run, automation run, journey instance, subscription

**Step**:
One node in an Automation's **ordered, linear** sequence; an Enrollment points at exactly one
current Step. Four kinds today: **send** (an email — it holds its _own copy_ of Message
content, per the marketing copy-at-author-time rule), **wait** (a delay before the next
Step), and **apply tag** / **remove tag** (change the enrolled Contact's Tags; they send
nothing and move straight on). The Enrollment's single "current step" pointer is deliberate: there is no branching,
no parallel paths, no per-step conditions yet. Conditional / branching steps are a real
future want but are **deferred until the sequence builder is real** — not modelled now.
_Avoid_: Action, node, block, stage

**Template** (email template):
A workspace-scoped, named, reusable piece of email content — a **starting point copied at
author time**. A Broadcast or Automation step takes a _copy_ of the template's content with no
reference back, so editing or deleting a Template never changes any already-authored or sent
message. A content library, not a live layout that propagates.

This copy-at-author-time rule is for **marketing** sends (Broadcast, Automation send step). A
**Transactional send** binds the _opposite_ way: it references a Template by id and renders
its current content with per-call variables at send time, so fixing a typo in a receipt
template instantly corrects every future receipt — without the customer redeploying their
app. Two deliberately different binding models, one per surface: marketing copies (sent
content is immutable history), transactional references (the template is live).
_Avoid_: Layout, theme, master, partial

**Message content** (a value, not an entity):
The reusable shape every email carries — a subject plus an MJML body (compiled to email-safe
HTML on send). A Template is the saved, named instance of it; a Broadcast and each email
Automation send step each hold their own copy; a Transactional send renders a _referenced_
Template's content with per-call variables. The same value across surfaces — copied for
marketing, referenced for transactional.
_Avoid_: Email body, content block

### Operational

**Integration**:
A workspace-scoped, **outbound** connection to an external sending provider — channel-agnostic
by design (email today via smtp/ses; sms reserved), with encrypted credentials and at most one
default per (workspace, channel). Broadcasts and Automations send _through_ an Integration (the
workspace default when unspecified). Sending is one half of a loop: bounces and complaints come
back via the Ingest hook and land in Suppression.
_Avoid_: Provider (alone), ESP, sender, connector

**Sending domain**:
A workspace-scoped, DNS-verified domain that a Contact's mail may be sent _from_ — the
authenticated **sending identity**, deliberately **independent of the Integration** (the
transport). sphericon owns a per-domain DKIM keypair and **signs every outbound message itself**
(native signing), so the same identity holds across SMTP, SES, and any future provider with no
re-verification. A `from_email` is a verified Sending domain plus a local part — never a free
string; the send path **hard-requires** the From domain to match a verified Sending domain on
all three surfaces (Broadcast, Automation, Transactional) and rejects otherwise.
`verified` is a **live** property: the gate is the published DKIM TXT matching our key (SPF and
DMARC are generated, shown, and checked but do **not** block). It is re-checked over time — if
the DKIM record disappears, the domain reverts to unverified, its sends block, and the owner is
notified. Distinct from a **Sending source** (that is the unsubscribe scope, not an identity).
Beyond `verified` (the DKIM send-gate) the domain carries a separate **bulk-ready** signal: a
published DMARC record (at least `p=none`), which Gmail/Yahoo hard-require of bulk senders. Its
absence **warns but does not block** — the send-gate stays DKIM-only (ADR 0010); bulk-readiness is
advisory-with-teeth (ADR 0012), because a missing `_dmarc` record gets otherwise-authed mail
rejected (a bounce) or spam-foldered by bulk-sender policy.
_Avoid_: From domain, verified domain, sender identity, SPF domain

**Complaint rate**:
The reputation-monitoring ratio `complaints / (sent − hard bounces)`, per (Workspace, **Sending
domain**), over a trailing time window. The denominator subtracts hard bounces as a cheap proxy
for _delivered_ (mailbox providers judge complaint rate over delivered mail, but sphericon only
records `email.sent` = accepted-by-provider — see **Sent**; a plain `/sent` denominator
understates, worst for the abusive senders that matter most). A **flow rate**: each event counts
by its own `occurred_at`, never linked back to the originating send, so it is always current and
needs no send↔feedback correlation. Derived from the `email.complained` / `email.sent` /
permanent `email.bounced` Events (source of truth), **not** a stored status — a core, user-facing
number computed live; the automated-suspension _thresholds_ that read it are EE (ADR 0007). Always
exposed as the triple `(numerator, denominator, rate)`; the rate is **undefined** below a volume
floor, never a scary ratio off a handful of sends.
_Avoid_: Spam score, sender score, reputation score (we expose honest ratios, not a black-box score)

**Bounce rate**:
The sibling reputation ratio `permanent_bounces / sent`, same (Workspace, Sending domain) grain
and trailing-window flow semantics as **Complaint rate**. Only **permanent** (hard) bounces —
the same ones that suppress and that mailbox providers penalize — count in the numerator;
**transient** (soft) bounces are excluded from the headline (at most a secondary counter), so
the metric measures list hygiene, not temporary blips. One hard bounce is thus used three
coherent ways: it suppresses (Suppression), it subtracts from the Complaint-rate denominator,
and it is the Bounce-rate numerator.
_Avoid_: Delivery-failure rate (that would fold in transient bounces)

**Engagement rates**:
The dashboard's open, click and click-to-open ratios over a chosen window (7, 30 or 90 UTC days).
A **send cohort**: the denominator is the Broadcast recipients sent in the window, and opens and
clicks are counted among that same cohort, so opened never exceeds sent and the rates stay within
[0, 1]. Not the **Complaint rate** or **Bounce rate**: those are deliverability flow rates per
Sending domain, counted from Events by their own time and never tied to a send.
_Avoid_: Reputation, deliverability (those name the Sending-domain rates)

**API token**:
A workspace-scoped, **scoped** bearer credential for the external `/api` surface — a public
`prefix` plus a hashed secret, carrying scopes, optional expiry, and revocation. Distinct from
a Key: a token is scoped, revocable, and expiring.
_Avoid_: API key, secret, password

**Key** (collect key / ingest key):
A bare, single-purpose secret string that identifies a workspace for one job — `collect_key`
(browser tracking ingestion) and `ingest_key` (routing inbound provider webhooks). No scopes,
not a bearer token. The deliberate counterpart to an API token.
_Avoid_: Token (for these), API key

**Operator** (platform operator):
A member of the **platform's** staff who acts _across_ all Workspaces (suspend an abusive
sender, impersonate for support). Deliberately **not** a User and **not** reached through a
Membership: a distinct identity with its own store and its own auth surface, holding **no**
Membership, so the "every query is scoped by a Workspace" invariant has no exception —
workspace-scoped code has no path that can return an Operator. A person who is both staff and
a customer holds two separate identities (an Operator _and_ a User), by design, for
least-privilege and clean audit. Created only by the `sphericon operator create` command (no
signup), it logs in with a password and a mandatory TOTP enrolled at first login, on a short
session of its own. Failed logins are rate limited, and a lost TOTP is reset only by
`sphericon operator reset-totp` (re-enrolment at next login), never through the web. Customers see its actions only as "sphericon staff". A SaaS/platform
concept, absent from a plain self-hosted install.
_Avoid_: Admin (that is a workspace Role), superuser, staff user, root

**Workspace suspension**:
A reversible Workspace state that **freezes all outbound sending** — every one of the three
send surfaces (Broadcast, Automation, Transactional) refuses while it is set — as a
platform reputation-protection measure against an abusive sender. It is _not_ a lockout:
login, dashboard reads, `/api` reads, and `/collect` tracking keep working, so the owner can
still see the suspension and appeal. Enforced in the **core** send path (the AGPL binary is
the only reliable choke point). It records **attribution**: the actor that set it (an
automated abuse detector — actor `system` — or a platform Operator) and a reason. The
automated path only fires above a minimum send volume (a rate is noise at low volume),
notifies the workspace owner, and is one-click reversible by an Operator. Suspension is the
_reputation_ freeze; a Billing hold is a separate money-driven freeze on the same core send
chokepoint — the two are independent reasons, never merged.
_Avoid_: Ban, lockout, disable, quota (suspension is not a billing state — see Billing hold)

**Operator console**:
The Operator's own web surface (a fourth API surface beside `/site`, `/api`, `/collect`) for
acting across Workspaces. It shows Workspace **metadata only** (identity, suspension state,
deliverability rates, send volume, Audit log) — never Contacts, content, or Events — and offers
suspend and unsuspend. Impersonation is out of scope. A SaaS/platform concept, absent from a
plain self-hosted install.
_Avoid_: Admin panel, back office

**Abuse detector**:
The automated actor (`system`) that watches each Workspace's **Complaint rate** and **Bounce
rate** and freezes sending through **Workspace suspension** when they cross a limit, above a
minimum send volume. A lower limit only **warns** the owner and freezes nothing. It counts only
events after the last time a suspension was cleared, never reverses a suspension itself (only an
Operator does), and skips a Workspace an Operator has exempted. A SaaS/platform concept.
_Avoid_: Spam filter, auto-ban

**Webhook endpoint** (outbound):
A workspace-scoped HTTP destination that **sphericon calls** when domain events occur — subscribes
to event types (empty = all) and signs each delivery (HMAC). Outbound only; the opposite of an
Ingest hook.
_Avoid_: Webhook (unqualified), callback

**Ingest hook** (inbound provider webhook):
The inbound endpoint that **receives** provider callbacks (e.g. SES bounce / complaint via SNS),
routed by the workspace's `ingest_key`. The opposite direction from a Webhook endpoint; it
feeds Suppression.
_Avoid_: Webhook (unqualified), inbound webhook

**User**:
A human who authenticates (email + password). A User reaches Workspaces through Memberships,
each carrying a Role — a Workspace is never owned by a single User directly.
_Avoid_: Account, member, customer, owner

**Second factor**:
A time-based one-time code (TOTP) a User enrolls in addition to their password, proving possession
of their authenticator app at login. Email-based and SMS factors do not exist.
_Avoid_: 2FA code, MFA, OTP (unqualified)

**Recovery code**:
One of a fixed set of single-use codes issued when a User enrolls a Second factor, accepted in
place of it when the authenticator is lost. Regenerating the set invalidates the old codes.
_Avoid_: Backup code

**Two-factor requirement**:
A Workspace-level policy, set by an Owner or Admin, that every User with a Membership in it must have a
Second factor; Users without one get a 7-day grace period before access is withheld.
_Avoid_: Enforced 2FA, mandatory MFA

**Second factor reset**:
An Owner or Admin removing another User's Second factor so they can re-enroll after losing both
authenticator and Recovery codes. Recorded as an Audit entry.
_Avoid_: Unlock, disable 2FA

**Login throttle**:
The growing delay applied to an account's login after repeated failures (password, Second factor
or Recovery code), during which even correct credentials are refused. It is never a permanent
lock, and unknown emails are throttled and answered identically.
_Avoid_: Lockout, ban, account lock (a hard lock would let an attacker deny a victim access)

**Membership**:
The join that grants a User access to a Workspace with a Role — Workspaces are reached through
Memberships, not owned by one User. Laid in now (rather than single-owner) because a multi-user
SaaS is planned and retrofitting access control onto single-owner is expensive.
_Avoid_: Team, seat, collaborator, ownership

**Role**:
A Membership's permission level in a Workspace (e.g. owner / admin / member). The exact set is a
follow-up; the structural decision is that access is role-scoped per Membership. A Workspace
always keeps at least one owner, and only an owner grants the owner Role or changes or removes
another owner's Membership.
_Avoid_: Permission, scope (scope is the API-token term)

**Audit entry** (the Audit log is the Workspace's append-only sequence of them):
An immutable record that an **actor** changed a Workspace's configuration or access — who
(a User, an API token, a platform Operator shown to the customer only as "sphericon staff", or
`system`), what (an action on a target), and when. It keeps a snapshot of the target's name (a Contact by id only), so
it stays readable after the target is deleted; secrets are never recorded as values, and
Contact changes name the fields changed, never their personal data. It covers the
_control plane_ only (membership and roles, credentials, domains, integrations, webhooks,
content and audience definitions, Contact edits and deletes, imports, exports, logins) — never
the data plane, whose facts are already Events and Outbound messages. Readable by owner and
admin. An Enterprise concept, absent from a plain self-hosted install.
_Avoid_: Event (that is a Contact/data-plane fact), activity, history, changelog

### Metering & billing

sphericon's core _measures_ billable activity; it does not _price_ it. Metering (what happened,
how much) is a domain concern; rating, plans, invoices, payment, and dunning are **not** — they
live in an external billing plane and never enter the core glossary.

**Billing period** (metering):
The calendar month, in UTC, over which usage is measured and a Usage snapshot is closed. The same
for every Workspace and owned by the core, not by the billing plane: the core closes a period
before the plane invoices it, so no usage arrives after an invoice is final. Any proration of a
part-month subscription is the plane's concern.
_Avoid_: Billing cycle, subscription anniversary

**Usage snapshot** (metering):
A per-(Workspace, Billing period, metric) materialized aggregate of billable activity, finalized
(made immutable) at period close — the billing-grade number a biller consumes. Distinct from the
raw Events it is computed from, which stay the source of truth for audit and dispute. Two
metrics, aggregated differently: `emails_sent` is a _sum_ of `email.sent` over all three send
surfaces, one per recipient, in a single total (a suppressed or held message is never sent, so
never counted); `contacts` is a **high-water-mark** (the peak reached during the period) of
identified Contacts — those with an email Destination, erased Contacts excluded — never an
instantaneous value, and never anonymous tracker Contacts. It materializes live Events for money
the same way a Broadcast recipient's rollup materializes engagement — the Events are truth, the
snapshot is the closed, reproducible figure. Carries no price.
_Avoid_: Meter, counter, invoice line, quota (quota is enforcement, not measurement)

**Live usage** (metering):
The same per-(Workspace, metric) figures for the Billing period still open: not final, with the
time it was last computed. Read from a worker or cache, never a recount on read, and counting
sends still in flight. It is what lets the billing plane notice a limit being approached before
the period closes; the Usage snapshot remains the only figure it invoices.
_Avoid_: Current usage, real-time usage

**Billing hold**:
A reversible Workspace state that **freezes all outbound sending** — all three send surfaces
(Broadcast, Automation, Transactional), exactly like Workspace suspension — but for a _money_
reason (non-payment, or a free plan's hard cap) rather than reputation. It is a **distinct cause
on the same core chokepoint**, never a repurposing of suspension: the send path asks one question
("may this Workspace send now?") answered by several independent freeze reasons, each stored
separately. Messages are **held, not failed**: they wait and go out when the hold lifts. Two
properties set it apart from suspension: the billing plane sets it only once dunning has ended
(the core has no grace period of its own; during dunning nothing is frozen, so a tenant's
password-reset mail is protected by the grace, not by message type), and it is cleared by
**payment (self-service)**, whereas suspension is cleared by appeal. A paid plan exceeding its
included volume is never held: overage is billed. The state and its lifecycle are an EE concept;
only the freeze check lives in the core send path.
_Avoid_: Suspension (that is the reputation freeze), quota, lockout, dunning (dunning is the
grace period, not the hold)

## Channels & future surfaces

The identity spine — **Contact + Visitor + Event** — is channel-agnostic and is the stable point
every current and future surface plugs into. Sending is modelled _per channel_: an Integration
has a channel, Send-eligibility is keyed by (channel, Destination), and Message content is
channel-specific. Email is the only built channel; SMS is reserved.

Deferred, additive surfaces — **not modelled until their shape is real**, and none should reshape
the core now:

- **On-site popup / form** — a capture-and-display surface; it feeds Events + Identify through
  the tracker. Additive entity, not a core change.
- **Messenger bot** — just another channel (a Destination is a chat id); covered by the
  channel-aware send model.
- **On-site chatbot** — a separate, two-way conversational context; shares only the identity
  spine. Explicitly out of the send/eligibility/sequence model.

## Anti-vocabulary (rejected concepts)

- **List / mailing list** — the old-school primitive that fused container + targeting +
  consent into one. Deliberately not used: pool = Contacts, targeting = Segment, consent =
  per-(Contact, Sending source) Unsubscribe. If a feature wants "the people on list X", model
  it as a Segment.
- **Tracking profile** — there is no separate person record for the behavioral/CDP side; the
  Contact _is_ the identity. (The old tracking profile is absorbed into Contact.)
- **Trait** — the schemaless CDP attribute. Not a separate concept: every non-core attribute
  is a typed, named Custom field (auto-created on first sight). We keep one governed attribute
  notion, not raw-traits-plus-promoted-fields.
- **Prospect** — a dropped, never-read flag. "Not yet a real contact" is not a stored term;
  it is the anonymous/un-identified state of a Contact, and mailability is derived anyway.
- **Contact status / subscribed flag** — send-eligibility is not a property of a Contact;
  see Send-eligibility above.
- **Blacklist** — internally the concept is Suppression. "Blacklist" in email means
  external DNSBLs (sender IP/domain reputation), a different concern not modeled here.
