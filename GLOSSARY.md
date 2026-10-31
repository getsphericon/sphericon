# sphericon

Marketing automation platform. The domain is workspace-scoped (multi-tenant): every
contact, event, segment, and tracking entity belongs to exactly one Workspace.

## Language

### Core

**Workspace**:
The tenant boundary. Every domain entity belongs to exactly one Workspace, and all
queries are scoped by it. Owns its own secret key for browser tracking (collect key).
_Avoid_: Account, tenant, organization, project

**Workspace name**:
The display title of a Workspace: free text (spaces, any case), changed at will by the Owner
and Admins. Not unique: two Workspaces, even of one User, may share a name.
_Avoid_: slug, handle (a Workspace has no human-readable address)

**Workspace id**:
The short opaque identifier of a Workspace that appears in its URLs and in the API. Assigned
once at creation and never changed; it is the only address a Workspace has.
_Avoid_: slug

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

**Contact source**:
How a Contact first came to exist in the Workspace: created by hand (`manual`), through the
external API, a batch import included (`api`), or identified by the tracker (`tracker`). An
Integration's ingest hook never creates a Contact. Set once at creation and never changed, even when the same person later arrives through another door. A Contact that
predates the field has no source. Not a **Sending source** (the unsubscribe scope).
_Avoid_: Origin, channel, acquisition source

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
always a Segment with a has-Tag Condition, so Tag does not reintroduce the rejected List model.
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
and carries denormalized identity snapshot fields for debugging. The link is a nullable
foreign key that is set to null when the Contact is deleted: an Event outlives the mutable
Contact, it never blocks or follows its deletion. The raw material behind behavioral segmentation. sphericon's own delivery and engagement facts (`email.sent`, `email.delivered`, `email.delayed`, `email.dropped`, `email.bounced`,
`email.complained`, `email.opened`, `email.clicked`, `email.unsubscribed`) are Events too — reserved,
system-generated actions — so engagement is segmentable through the very same machinery as
customer-tracked actions, with no parallel event stream.
_Avoid_: Activity, log entry, signal

**Import**:
Bringing records from a Workspace's earlier system into the Workspace, repeatably: sending the
same record again changes nothing. Contacts and Events are imported differently; Events are a
**Backfill**.
_Avoid_: Migration, sync

**Backfill**:
An **Import** of Events that happened before the Workspace came to sphericon, recorded with the
time they really happened. To segmentation a backfilled Event is an ordinary Event; to Automations
and webhooks it is history and never a trigger.
_Avoid_: Historical import, replay, silent event

**Erasure**:
Removing a Contact's personal data at the Workspace's request (GDPR Art. 17). It is what deleting
a Contact _is_ — there is no softer delete that keeps personal data. The Contact, its Visitors,
Custom field values, Tags and Confirmation are removed, as are its customer-tracked Events and the Captured messages sent to its Destinations; the
reserved `email.*` Events and delivery records stay as anonymous rows, so deliverability and
metering counts do not change. What survives is the minimum that keeps the refusal honored: the
Unsubscribe rows and the active Suppression rows for its Destinations (lifted ones refuse
nothing and go). It leaves a PII-free `contact.erased`
Event, stored without any Contact reference (its webhook delivery carries the customer's own
subject_id so downstream copies can be erased). A person who later reappears (Identify, import) is a new Contact and
is not blocked; surviving opt-outs still apply to them. The Workspace is the controller and sphericon
the processor, so Erasure is a Workspace action, never a data-subject self-service form. A request
names a person by address, so the Workspace's owners and admins can also start it from an address
(a **Privacy request**), even when no Contact holds that address any more: then only the delivery
records of the address are anonymized. A Privacy request starts with a lookup that shows counts
only, never content (Captured messages are counted too); "nothing held" is a valid result (zeros, not a not-found). Erase is offered
only while a Contact or delivery records hold the address: opt-outs alone are not erasable.
_Avoid_: Anonymization (that is only what happens to the delivery rows), forget, purge (purge is
bulk retention policy, a separate concern)

**Data export** (subject access):
A machine-readable bundle of everything held about one Contact (GDPR Art. 15/20): the Contact,
its attributes, Tags, Visitors, Events, opt-outs and delivery metadata — not rendered message
bodies.
_Avoid_: Backup, dump, contact export (that is the bulk list export)

**Retention**:
How long an Event is kept before it is deleted. Events split by purpose: **evidentiary** Events
(consent proof such as `marketing.confirmed`, hard bounces and complaints, unsubscribes)
are the audit and dispute record and are never deleted by age; **analytical** Events (opens, clicks,
deliveries, customer-tracked actions) are deleted once older than the instance's retention window,
so segment conditions cannot look back past it.
_Avoid_: TTL, expiry, purge, archive

**Condition**:
A tree of tests (a combinator over nested groups) on a Contact's fields, custom fields, Tags and
past Events, and, when a Rule evaluates it, on the triggering Event's own properties. Evaluated
live against current data, never materialized. One shared grammar: a Segment is a named Condition
and a Rule holds one.
_Avoid_: Filter, query, predicate, rule (a Rule is something else)

**Segment**:
A named, reusable Condition: "which contacts match these conditions", the one targeting
primitive. Membership is never materialized and shifts as data changes. There is no other
segment kind; a segment is always a Condition.
_Avoid_: List, group, filter, snapshot, and "audience" as a name (it stays prose for who a Broadcast reaches)

**Rule**:
A workspace-scoped, stateless reaction: when any of its **Trigger events** is recorded for a
Contact and the Rule's Condition holds for that Contact and Event, its Actions run at once, in
order. No delay, no Enrollment and no email step; those belong to an Automation. Fires on every
matching Event, only going forward, and at most once per Event (a **Firing** records what ran).
An Event with no Contact, a machine Event, and an Event caused by a Rule's own Action trigger no
Rule. Actions today: apply Tag, remove Tag, HTTP call, enroll in an Automation. What it does is
its live **Rule version**; an edit writes a draft, and only **Publish** makes it live.
_Avoid_: Workflow, flow, hook, automation (an Automation is sequential and stateful)

**Order**:
The read model of one purchase a Workspace's shop reported through order Events, one per `order_id`:
its total, its currency, its refunds and whether it was cancelled. Created and changed only by Events;
it holds no catalogue and no payment state.
_Avoid_: Purchase, transaction, sale

**Revenue**:
The net amount of Orders in the Workspace's reporting currency: each Order's total minus its refunds,
a cancelled Order counting nothing. **Attributed revenue** is the part credited to a Broadcast or an Automation.
_Avoid_: Sales, turnover, GMV

**Reporting currency**:
The one currency in which a Workspace's revenue is summed. An Order in another currency is kept but
counted in no sum.
_Avoid_: Base currency, default currency

**Attribution window**:
How long after a Contact clicks an email of a Broadcast or an Automation an Order of that Contact is credited to it. The
Workspace edits it; 5 days until set. The last click inside it wins, and an open credits nothing.
_Avoid_: Lookback, conversion window

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
Removing one is **lifting** it: the row stays with `lifted_at` and no longer blocks sends.
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
sent / failed). Its audience is a Segment (or all Contacts). Scheduling takes a **copy of the
Segment's Condition**, so a later edit of the Segment does not change who the Broadcast reaches;
the copy is resolved and **frozen into Recipients at send time**, then filtered per-recipient by
Send-eligibility. Sends under the
shared "broadcasts" unsubscribe scope. There is no "active contacts" audience — eligibility is
derived, not a contact status. "Audience" is prose for whoever a Broadcast reaches; the thing
that picks them is always a Segment, and names say Segment.
_Avoid_: Campaign, newsletter blast, mailshot, email blast

**Broadcast recipient**:
The per-(Broadcast, Contact) record — the frozen audience snapshot taken at send
time. It holds no delivery state of its own: its send ends in exactly one Outbound message
(sent / skipped / failed), and it is _pending_ until then. It is also the home of a denormalized
**rollup** of engagement (opened / clicked / unsubscribed, each set once) for per-broadcast
stats; opens and clicks are also the `email.opened` / `email.clicked` Events, which are the
source of truth, so the rollup is a convenience view. Delivery figures (delivered, bounced, complained) are read from
the Outbound message's delivery result, never counted from Events. A Broadcast's figures (recipients, sent,
skipped, failed, pending, opened, clicked, unsubscribed) are derived from its recipients and
their messages and never stored, so they are live while it sends. Contrast Segment, which is a
live query and never materialized.
_Avoid_: Send log, delivery row

**Outbound send**:
The one act of sending a single email on any surface (Broadcast, Automation step,
Transactional): Send-eligibility, the Workspace freeze check, the Sending-domain gate,
rendering, the unsubscribe footer and headers, DKIM signing, the provider call, and the record of
the result. Every surface goes through it; none re-implements any part.
_Avoid_: Delivery pipeline, send path (unqualified), mailer

**Test send**:
A Broadcast's author sending it to one address they name, to see the mail before it goes out. It
skips Send-eligibility and leaves no Outbound message or `email.sent` Event, so it never shows in
Activity or the rates. It goes through the Integration the Broadcast sends through (its own, else
the Workspace's default). While none of the Workspace's Sending domains is verified and that
Integration delivers outside (or there is none to send through), it goes from the System sender, and
only to the address of the User who asks. Through a Test inbox, which delivers nowhere outside, it
goes through that Integration and is captured, and the editor says it was not sent to that address.
_Avoid_: Preview (that is rendering without sending), dry run

**System sender**:
The Instance's own sending identity for platform mail to Users (welcome, email verification,
password reset, invitations, the unverified-domain notice) and for a Test send from a Workspace
with no verified Sending domain. It belongs to no Workspace and is absent on an Instance with no
outbound mail.
_Avoid_: No-reply sender, platform mailer

**Outbound message**:
The durable record of one Outbound send — which surface and which of its records it belongs to,
the Sending source (none for Transactional), the Sending domain, the provider's message id, the
outcome, and the delivery result: the first Delivery notice of each kind that arrives stamps it once
(delivered, bounced, complained), and a stamp is never removed, so reports of old sends do not shrink
when Events expire. Claimed before the provider is called, under a per-logical-send idempotency key, so a
retry never sends twice. Holds provenance and outcome, never rendered content. A Broadcast
recipient points at its Outbound message; it does not replace it.
_Avoid_: Send log, delivery row, email record

**Outcome** (of an Outbound send):
Either **per-recipient** and final for that destination — _Sent_, _Skipped_ (Suppression,
Unsubscribe, or missing Confirmation), or a permanent failure of that one message — or
**per-source** and reversible: a **Hold** (Workspace suspension, Staff hold, an unverified
Sending domain, no Integration, or a provider that has stopped the customer's account) or a **Deferral** (the Integration's Provider limit or the
Workspace's Send cap is spent), after which the same messages may still be sent.
_Avoid_: Status, result (unqualified)

**Provider limit**:
How fast an Integration may send, mirroring what the customer's own provider account allows: a
maximum messages per second and a maximum messages per rolling 24 hours. The provider reports
its own where it can (SES) and a user may set one; the lower of the two applies (the user's on a
tie), and with neither the Integration is unlimited. It protects the customer's provider account,
not the platform. Applies to Broadcast and Automation sends; Transactional sends are never
delayed or deferred by it but still count against the 24-hour total.
_Avoid_: Send rate limit, send limit, throttle (as a noun for the limit), quota

**Send cap**:
The most messages a Workspace may send per rolling 24 hours across all its Integrations, set by
a Staff member as a platform reputation-protection measure and not editable by the Workspace.
Only a `saas` Instance has it: a self-hosted Instance does not limit what its Workspaces send. It
slows sending rather than stopping it: a spent Send cap is a Deferral, and the Workspace sees the
cap and that the platform set it. The Instance may give every new Workspace one by default; with
none set, a Workspace has no Send cap. Transactional sends count against it but are never delayed
by it. Distinct from the Provider limit, which the customer's provider imposes, and from Workspace
suspension, which stops sending outright.
_Avoid_: Quota, plan limit, allowance, throttle, ceiling (unqualified)

**Deferral**:
The reversible, per-source Outcome of an Outbound send that found its Integration's Provider
limit or its Workspace's Send cap spent: the message is not consumed and is retried when capacity returns. Unlike a _Hold_,
nothing is wrong with the source — it is simply busy — so it carries no hold reason and a
Broadcast stays _sending_ with a progress estimate instead of a blocked state.
_Avoid_: Throttled, rate-limited (as an outcome), delay

**Hold**:
The reversible, per-source Outcome that stops sending without consuming recipients: a Broadcast
pauses with its remaining recipients pending, Automation Enrollments wait, a Transactional request
is refused. Distinct from _Skipped_, which is final for one destination.
_Avoid_: Block, failure, ban

**Sent** (vs delivered):
"Sent" means a message was _accepted by the email provider_ — not that it reached the inbox
(`email.sent`). **Delivered** means the receiving mail server accepted it, as the provider reports
in a Delivery notice (`email.delivered`); it is still not "opened" or "in the inbox". A
broadcast's send rate is accepted-by-provider ÷ targeted.
_Avoid_: Delivered (for the accepted-by-provider sense), Sent (for the receiving-server sense)

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
Automation is its own unsubscribe Sending source. What it does is its **live version**: an edit
writes a draft, and only **Publish** makes it live. Publishing starts enrolling; deactivating
stops _new_ enrollments but lets in-flight Enrollments finish (deactivate ≠ halt). Removing
one is **Archive** (a **Discard**) once no Enrollment is active, which hides it and enrolls
nobody while its Enrollments stay; only an Automation with no Enrollments at all is purged.
_Avoid_: Workflow, flow, journey, drip, campaign

**Automation version**:
One copy of an Automation's definition: its Trigger, its Integration and its Steps. The **live
version** is the one new Enrollments begin on; the **draft** is a newer version that edits are
written into, rewritten in place until it is published. A live version is never rewritten, and an
Enrollment stays on the version it began on. The name belongs to the Automation, not to a version.
_Avoid_: Revision, snapshot

**Rule version**:
One copy of a Rule's definition: its Trigger events, its Condition and its Actions. Live and draft
as for an Automation version; a Firing runs the version that fired.
_Avoid_: Revision, snapshot

**Versioned definition**:
An object whose definition is kept in versions, a **live version** and a **draft**, and changes only
by **Publish**; its name, status and archiving stay on the object. Today the Automation and the Rule.
They share one lifecycle: an edit writes the draft, Publish makes it live, **Revert** drops it,
Deactivate stops the object, and Archive makes it inactive too, so restoring it never re-arms a send.
A Broadcast is not one: a scheduled Broadcast holds a copy of its Condition instead.
_Avoid_: Versioned entity, revisioned object, publishable

**Publish**:
The send-class act that makes the draft of an Automation or a Rule its live version and makes it
active; with no draft, it makes the live version active again. The only way the definition of a
live Automation or Rule changes.
_Avoid_: Activate, launch, go live, deploy

**Revert**:
Dropping the draft of an Automation or a Rule, so that its live version is again the latest. It
changes nothing that sends.
_Avoid_: Discard (that is the soft-delete mechanism), reset, undo

**Trigger**:
The single Event action that enrolls a Contact into an Automation (e.g. "contact.created", a
custom event). Not a separate entity — one Trigger per Automation version. Matched by the Event's
resolved identity, not by email.
_Avoid_: Hook, entry condition

**Enrollment**:
One Contact's membership-and-progress in one Automation — enrolled at most once ever
(re-enrollment is deliberately out of scope). Stands on one **Automation version** (the one live when
it began) and holds the current step in it, and a terminal state:
`completed`, `failed`, or **`exited`** (left early). An unsubscribe from the Automation has two
effects from one action — it records the durable opt-out (scope `automation:<id>`) _and_ moves
the active Enrollment to `exited`; suppression and hard bounces likewise exit the Enrollment.
A run never silently keeps walking steps while skipping every email.
_Avoid_: Run, automation run, journey instance, subscription

**Step**:
One node in an Automation's **ordered, linear** sequence; an Enrollment points at exactly one
current Step. Five kinds today: **send** (an email — it holds its _own copy_ of Message
content, per the marketing copy-at-author-time rule), **wait** (a delay before the next
Step), **apply tag** / **remove tag** (change the enrolled Contact's Tags; they send
nothing and move straight on), and **HTTP step** (below). The Enrollment's single "current step" pointer is deliberate: there is no branching,
no parallel paths, no per-step conditions yet. Conditional / branching steps are a real
future want but are **deferred until the sequence builder is real** — not modelled now.
_Avoid_: Action, node, block, stage

**HTTP step**:
A Step kind that **sphericon calls** a third party's URL (a Slack, Zapier or CRM hook) for the
enrolled Contact. It holds its _own_ URL, headers and a templated JSON body (the author-time copy
rule again), and signs the request only when its author sets a secret. The Enrollment moves on
after a 2xx; when the retries are spent it becomes `failed`, recording the status or the kind of
error and the first 1 KiB of the response, never skipping the step. Distinct from a Webhook
endpoint: that one is subscribed to events, this one runs when an Enrollment reaches it.
_Avoid_: Webhook step, action

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
The reusable shape every email carries — a subject plus a body of email-ready HTML with merge
tags. A body written in the editor is built from the editor's document, which is kept with it; a
body that arrived as HTML (from the API) has no document and opens as source. A Template is the saved,
named instance of it; a Broadcast and each email Automation send step each hold their own
copy; a Transactional send renders a _referenced_ Template's content with per-call variables.
The same value across surfaces — copied for marketing, referenced for transactional.
_Avoid_: Email body, content block

**Text part** (derived, never authored):
The `text/plain` alternative a sent email carries beside its HTML, derived from the _final_
HTML of that recipient (merge tags resolved, links already tracked, unsubscribe footer included)
and never edited on its own. A message is always HTML: a "plain" letter is an ordinary message
written without design, not a separate format or entity.
_Avoid_: Plain-text email, text template, text version

**Brand kit**:
A Workspace's visual identity as data — colors, fonts, logo, button style and footer. It sets
the palette the editor offers and what an agent is told to use. Its values are written into a
message when the message is authored, so changing the Brand kit never changes a message that is
already written.
_Avoid_: Theme, style guide

### Operational

**Discard**:
Taking a row out of working lists while keeping it, by stamping `discarded_at`; **Undiscard**
clears the stamp. It is the one mechanism behind what the UI calls Archive (a Broadcast, an
Automation, a Segment, a Rule: a list view of the archived ones restores them), Remove (an
Integration, a Sending domain: no view of the removed, restorable through the API only) or Delete
(a Workspace, restorable for its window). A Sign-up item is archived for good: it stays listed,
marked archived, and is never restored. A discarded row is hidden from
lists and from new work, but not from history: reports still count it. A row that other live
work depends on cannot be discarded. **Purge** is the physical removal, allowed only for what
has no history (a draft Broadcast, an Automation with no Enrollments) and for erasure. The
raw, undiscardable fact rows (Events, Outbound messages, Audit entries, Suppressions) are never
discarded.
_Avoid_: Soft delete, archive (as the mechanism), trash, tombstone (a tombstone is the row an
erased Workspace leaves)

**Integration**:
A workspace-scoped, **outbound** connection to a sending provider, external or built in (the Test
inbox) — channel-agnostic by design (email today via smtp/ses/inbox; sms reserved), with encrypted
credentials where the provider has any and at most one default per (workspace, channel). Broadcasts and Automations send _through_ an Integration (the
workspace default when unspecified). Sending is one half of a loop: bounces and complaints come
back via the Integration's Ingest hook and land in Suppression.
_Avoid_: Provider (alone), ESP, sender, connector

**Test inbox**:
An Integration provider that sends nothing outside: the messages of an Outbound send through it are
kept as Captured messages and read in the app. It is chosen, set as default and rate-limited like any
other Integration, and the send path has no case of its own for it. The way to see what a Workspace's
mail would look like and do before any real provider or DNS exists.
_Avoid_: Sandbox (a provider's own test mode), Mailpit, fake provider

**Captured message**:
The kept copy of one message sent through a Test inbox: its headers, HTML and text parts and the links
in it, owned by the Workspace and the Integration. It is the only place rendered content is stored (an
Outbound message holds none), expires with the Test inbox's retention and is never a record of a send.
_Avoid_: Inbox message, email log

**Tracking domain**:
The hostname `track.<sending domain>`, a CNAME to the Instance's edge, under which the links in mail
sent from that Sending domain (click, open, unsubscribe, confirm) are built, so recipients see the
sender's own domain. Set up with the Sending domain's DNS records but verified on its own; it serves
only those link paths and the first-party tracker (`/t.js`, `/collect`, for its own Workspace's collect
key, ADR 0059), and until it is serving, new mail uses the Instance's own host.
_Avoid_: Click domain, link domain, custom domain (unqualified)

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
The reputation-monitoring ratio `complaints / delivered`, per (Workspace, **Sending domain**), over a
trailing time window, where `delivered` is the count of `email.delivered` Events (mailbox providers
judge complaint rate over delivered mail). A **flow rate**: each event counts by its own
`occurred_at`, so it is always current and never needs a Delivery notice to be linked back to its
send. Derived from the `email.complained` / `email.delivered` Events (source of truth), **not** a
stored status — a core, user-facing number computed live; the automated-suspension _thresholds_ that
read it are EE (ADR 0007). Always exposed as the triple `(numerator, denominator, rate)`; the rate is
**undefined** below a volume floor, never a scary ratio off a handful of sends, and so for a Preset
that reports no delivery.
_Avoid_: Spam score, sender score, reputation score (we expose honest ratios, not a black-box score)

**Bounce rate**:
The sibling reputation ratio `hard_bounces / sent`, same (Workspace, Sending domain) grain
and trailing-window flow semantics as **Complaint rate**. Only **hard bounces** —
the same ones that suppress and that mailbox providers penalize — count in the numerator;
**soft bounces** are excluded from the headline (at most a secondary counter), so
the metric measures list hygiene, not temporary blips. One hard bounce is thus used two
coherent ways: it suppresses (Suppression) and it is the Bounce-rate numerator.
_Avoid_: Delivery-failure rate (that would fold in soft bounces)

**Engagement rates**:
The open, click and click-to-open ratios of the analytics reports over a chosen window (7, 30 or 90 **Report days**).
A **send cohort**: the denominator is the Broadcast recipients sent in the window, and opens and
clicks are counted among that same cohort, so opened never exceeds sent and the rates stay within
[0, 1]. Not the **Complaint rate** or **Bounce rate**: those are deliverability flow rates per
Sending domain, counted from Events by their own time and never tied to a send.
_Avoid_: Reputation, deliverability (those name the Sending-domain rates)

**Workspace time zone**:
The IANA time zone a Workspace reads its reports in (UTC until set). It decides where a **Report day**
begins and ends and which hour an event falls in. Not a Contact's own time zone, which is about
where that person is.
_Avoid_: Locale, region

**Report day**:
A calendar day in the **Workspace time zone**: the one bucket every analytics report counts by, so
the dashboard, the engagement rates and the hour-of-day report agree on what "today" and "9 a.m."
mean. Not a UTC day unless the Workspace time zone is UTC.
_Avoid_: UTC day

**API token**:
A workspace-scoped, **scoped** bearer credential for the external `/api` surface — a public
`prefix` plus a hashed secret, carrying scopes, optional expiry, and revocation. Distinct from
a Key: a token is scoped, revocable, and expiring. It belongs to the Workspace and outlives the people
who minted it; a User's own grant to an application is a Connection, and a User's own key is a
Personal token.
_Avoid_: API key, secret, password

**Personal token**:
A User's own bearer credential for the external `/api` surface in one Workspace, for the scripts
and tools a person runs. It belongs to that User's Membership, carries scopes chosen within the
User's Role, can do no more than the Role allows at the moment of each request, and ends with the
Membership.
_Avoid_: PAT, personal API key, API token (that is the Workspace's)

**Connection**:
The access a User gave an application (an MCP client such as Claude) to one Workspace by
consenting to it. It belongs to that User's Membership, can do no more than the User's Role
allows at the moment of each request, and ends with the Membership. Like the User's own session,
it is refused while the User has a required acceptance pending or the Workspace's Two-factor
requirement withholds them.
_Avoid_: OAuth grant, authorized application, API token (that is the Workspace's)

**Key** (collect key / ingest key):
A bare, single-purpose secret string for one job — a Workspace's `collect_key` (browser tracking
ingestion) and an Integration's ingest key (the path of its Ingest hook, so it also names the
Workspace). No scopes, not a bearer token. The deliberate counterpart to an API token.
_Avoid_: Token (for these), API key

**Staff member**:
A person who runs the Instance (its operator; on the SaaS, the platform's staff) and acts on it
as a whole: its own settings, and with the Enterprise features _across_ all Workspaces (suspend
an abusive sender, impersonate for support). Deliberately **not** a User and **not** reached through a
Membership: a distinct identity with its own store and its own auth surface, holding **no**
Membership, so the "every query is scoped by a Workspace" invariant has no exception —
workspace-scoped code has no path that can return a Staff member. A person who is both staff and
a customer holds two separate identities (a Staff member _and_ a User), by design, for
least-privilege and clean audit. Created only by the `sphericon staff create` command (no
signup), it logs in with a password and a mandatory TOTP enrolled at first login, on a short
session of its own. Failed logins are rate limited, and a lost TOTP is reset only by
`sphericon staff reset-totp` (re-enrolment at next login), never through the web. Customers see its actions only as "sphericon staff". Every
Instance has one, self-hosted or not.
_Avoid_: Operator (the former name), Admin (that is a workspace Role), superuser, staff user, root

**Workspace deletion**:
An Owner's removal of a Workspace, confirmed by typing its name. It is **soft**: for 30 days the
Workspace is hidden from every Member's list and frozen like a suspension (nothing is sent), and
the Owner can restore it; after that a background job erases its data, its Memberships go and
its tracking hosts are released. Users are never deleted with it. Any Workspace can be deleted,
the last one included: a User with no Workspace is taken to create one.
_Avoid_: archive, close

**Workspace suspension**:
A reversible Workspace state that **freezes all outbound sending** — every one of the three
send surfaces (Broadcast, Automation, Transactional) refuses while it is set — as a
platform reputation-protection measure against an abusive sender. It is _not_ a lockout:
login, dashboard reads, `/api` reads, and `/collect` tracking keep working, so the owner can
still see the suspension and appeal. Enforced in the **core** send path (the AGPL binary is
the only reliable choke point). It records **attribution**: the actor that set it (an
automated abuse detector — actor `system` — or a platform Staff member) and a reason. The
automated path only fires above a minimum send volume (a rate is noise at low volume),
notifies the workspace owner, and is one-click reversible by a Staff member. Suspension is the
_reputation_ freeze; a Staff hold is a separate freeze set from outside for another reason,
on the same core send chokepoint — the two are independent reasons, never merged.
_Avoid_: Ban, lockout, disable, quota (suspension is not an entitlement state — see Staff hold)

**Staff console**:
The Staff member's own web surface (a fourth API surface beside `/site`, `/api`, `/collect`). On
every Instance it holds the Instance's own settings: its Sign-up items, the Telemetry report and its
notice, and the expiry of the License key.
With the Enterprise feature it also acts across Workspaces: it shows Workspace **metadata only**
(identity, suspension state, deliverability rates, send volume, Audit log) — never Contacts,
content, or Events — and offers suspend and unsuspend. Impersonation is out of scope.
_Avoid_: Admin panel, back office

**Abuse detector**:
The automated actor (`system`) that watches each Workspace's **Complaint rate** and **Bounce
rate** and freezes sending through **Workspace suspension** when they cross a limit, above a
minimum send volume. A lower limit only **warns** the owner and freezes nothing: the warning is
computed from the rates, never stored, and the owners get one email per tier per episode (an
episode ends when the Workspace leaves the tier). It counts only
events after the last time a suspension was cleared, never reverses a suspension itself (only an
Staff member does), and skips a Workspace a Staff member has exempted. A SaaS/platform concept.
_Avoid_: Spam filter, auto-ban

**Webhook endpoint** (outbound):
A workspace-scoped HTTP destination that **sphericon calls** when domain events occur — subscribes
to event types (empty = all) and signs each delivery (HMAC). It remembers its own failures and
**disables itself** on a 410 or after persistent failure (a 429 is not a failure), and its owners
are told by email. Outbound only; the opposite of an Ingest hook.
_Avoid_: Webhook (unqualified), callback

**Export stream** (Enterprise):
A Workspace's rows of one kind (Events, Outbound messages, Contacts, Erasures, Audit entries) that
an external consumer reads page by page with a cursor, to keep its own copy in a warehouse such as
ClickHouse. A changed row is delivered again, so the consumer keeps the latest version by id; a
row that was removed is announced by the **Erasures** stream, never by silence. Delivery is
at-least-once and carries no ordering guarantee across commits: the consumer overlaps its pages and
deduplicates. Pull, the opposite direction from a Webhook endpoint, which pushes.
_Avoid_: Sync, replication, feed, data export (that is one Contact's subject access bundle)

**Ingest hook** (inbound provider webhook):
The inbound endpoint of one Integration that **receives** its provider's callbacks (bounce,
complaint, delivery, delay, drop), addressed by the Integration's ingest key and decoded by the Integration's Preset. The
opposite direction from a Webhook endpoint; it feeds Suppression.
_Avoid_: Webhook (unqualified), inbound webhook

**Preset**:
A declarative description of one provider's callbacks — how its signature is verified and how its
payload becomes Delivery notices. Built into the binary (ses, postmark, mailgun, generic) or
supplied by the Self-hoster for the whole Instance, never by a Workspace.
_Avoid_: Adapter, plugin, provider (alone)

**Delivery notice**:
The one normalized shape every Preset produces: a Destination, its kind (bounce, complaint,
delivery, delayed or dropped), for a bounce whether it is hard or soft, and, when the provider
returns it, the provider's message id, which links the notice to the Outbound message it is
about. Only hard bounces and complaints enter Suppression. A notice with no message id stays
at the level of the address.
_Avoid_: Delivery failure (delivery and delay are not failures), Bounce event (it also covers complaints)

**Hard bounce**:
A bounce the receiving side reports as permanent: the Destination does not exist or refuses mail
for good. It suppresses at once (Suppression) and is the numerator of the **Bounce rate**.
_Avoid_: Permanent bounce, permanent failure (the industry says hard; one word for one thing)

**Soft bounce**:
A bounce the receiving side reports as temporary: a full mailbox, a server that is down, a
message that is too large. It does not suppress and does not count in the **Bounce rate**, but
the Outbound message still carries it as a bounce.
_Avoid_: Transient bounce, temporary failure (the industry says soft; one word for one thing)

**Machine engagement**:
An open or click made by software rather than the recipient: a mail client's privacy proxy (Apple
Mail Privacy Protection), a security scanner or a link prefetcher. It is still recorded as an
`email.opened` / `email.clicked` Event, flagged `machine`, but it never counts toward open or click
figures, never sets a Broadcast recipient's rollup and never starts an Automation. Detected from
the request's user-agent and source address, neither of which is stored. Detection is a partial
signal, not proof: a proxy that hides itself is not caught.
_Avoid_: Bot open, false open

**User**:
A human who authenticates (email + password). A User reaches Workspaces through Memberships,
each carrying a Role — a Workspace is never owned by a single User directly.
_Avoid_: Account, member, customer, owner

**Sign-up**:
A person creating a User for themself, with a password or through a Sign-in provider. Sign-up is
open on every Instance (ADR 0063). The new User stays an Unverified User until its address is proven.
_Avoid_: Registration, register, signup (one word)

**Leaving**:
A User's own removal of their Membership in one Workspace, with no action by an Owner. The last
Owner cannot leave: another Member must become Owner first, or the Workspace is deleted. A User
left with no Workspace is taken to create one.
_Avoid_: Quit, unjoin, self-removal (removal is what an Owner does to another Member)

**User deletion**:
A User's own immediate and irreversible removal of themself: their Memberships, Linked
identities, Second factor, Recovery codes and Passkeys go, every session ends, and the address can sign up
again as a new User. Refused while the User is the only Owner of any Workspace. Invitations the
User sent stay valid without a sender; Audit entries that name the User keep the id and the name
snapshot. It never touches a Contact or a Staff member (a Contact has its own Erasure).
_Avoid_: Account deletion, closing an account, deactivation, Erasure (that is a Contact's)

**Verified email**:
A User's email address whose control has been proven by following a link sent to it (a verify
link, an accepted Invitation, or a confirmed email change). Until then the User is an
**Unverified User**: it holds no session, no Membership and no Workspace, and its password, if
any, is not yet usable. A password chosen before the address is proven stays only if the link is
opened in the **Sign-up browser**; otherwise it is set again through the link, so whoever
proves the address is the one who sets the password.
_Avoid_: Confirmed email, activated account

**Sign-up browser**:
The browser that submitted a Sign-up. It holds a random value that the verify link also
carries (hashed), so the link can tell it is being opened where the Sign-up happened.
_Avoid_: Registration browser, same device, trusted browser

**Join domain**:
A Workspace-scoped, DNS-verified domain whose Users may join the Workspace automatically. An Owner
adds it and proves control by publishing a TXT record at `_sphericon-join.<domain>` whose value is
a random token; it stays verified only while the record is published. Any Workspace may hold a
pending claim of a domain, and only one holds it verified across the Instance: a Workspace that
proves a domain another one holds re-checks the holder's record first, and the domain moves to it
only when that record is gone. Removing a Join domain removes only the claim, never a Membership.
A domain is exact: `acme.com` does not cover `eu.acme.com`.
_Avoid_: Allowed domain, trusted domain, SSO domain

**Domain join**:
A Membership created because its member proved an address on a Workspace's verified Join domain
(ADR 0051): Role `member`, source `domain`. A Workspace has one switch for it, off by default, that
only an Owner changes. It happens at the moment an address becomes a Proven email and never later:
an address proven before the switch was turned on joins nobody, and turning it on adds and invites
no one. It ignores a suspended Workspace or one in its deletion window, and sends no mail. It is audited as `membership.domain_join`
with the system as the actor.
_Avoid_: Auto-join, domain capture, domain-based signup

**Sign-in provider**:
An external service that proves a person's email address so they can sign up or log in without a
password of ours: Google, GitHub. The Instance operator enables one by setting its OAuth client id
and secret; a provider replaces only the password step (an enrolled Second factor, the Login throttle
still apply). A person's own account at a provider, not an organization's
identity system: per-Workspace federation stays Enterprise (ADR 0014).
_Avoid_: SSO, social login, OAuth provider (that is the OAuth server for MCP clients)

**Linked identity**:
A Sign-in provider and the provider's immutable subject for one person, attached to a User. A
sign-in finds the User by this pair and never by email, so a person who changes the address at
the provider still reaches the same account. When a sign-in with an unknown subject carries a
trusted email that a Verified User holds, the identity is linked silently and the owner is mailed a
notice with a "this was not me" link that unlinks it and ends every session. If that address was
never proven (an install with verification off marks it Verified without proof), the link also
clears the password, so whoever proves the address is the one who controls it.
_Avoid_: Social account, connected account

**Pending sign-up**:
What a Sign-in provider proved about a person who has no account yet (the subject, a trusted email,
a name, the Invitation the flow began from), kept for ten minutes while the Instance asks its Sign-up
items. No User exists until the person answers them: their answers create the User, its Linked
identity, its first Workspace and the Consent records together; declining deletes it, and a job
deletes one left unanswered. A known Linked identity never has one.
_Avoid_: Draft account, half-registered user

**Taken address**:
An email that already belongs to a User. Nobody but its owner may learn that it is taken: a
request that would hit it gets the same answer as one that would not, and the owner is told by
mail. The one exception is an install that turned email verification off.
_Avoid_: Duplicate email, existing account

**Unverified User**:
A User whose email is not yet a Verified email: a Sign-up awaiting its verify link. It
cannot log in, even with the password it signed up with, owns nothing and is not attached to any Workspace. A daily job deletes one that
holds no Membership once it is older than `SPHERICON_UNVERIFIED_USER_RETENTION_DAYS` (default 7), freeing
its address; a pending Invitation to that address is kept.
_Avoid_: Pending account, inactive user

**Second factor**:
A time-based one-time code (TOTP) a User enrolls in addition to their password, proving possession
of their authenticator app at login. Email-based and SMS factors do not exist.
_Avoid_: 2FA code, MFA, OTP (unqualified)

**Recovery code**:
One of a fixed set of single-use codes issued when a User enrolls a Second factor, accepted in
place of it when the authenticator is lost. Regenerating the set invalidates the old codes.
_Avoid_: Backup code

**Passkey**:
A credential a User registers on a device or security key to log in without typing an email or a
password. A login with a Passkey always proves the device and the person (biometric or PIN), so it
replaces both the password and the Second factor. A User may hold several, each named. It is a
way to prove who the User is, not a Sign-in provider's proof of an address and not an
organization's identity system.
_Avoid_: Security key (a device that can hold one), WebAuthn credential, biometric login

**Two-factor requirement**:
A Workspace-level policy, set by an Owner or Admin, that every User with a Membership in it must sign in
with a Second factor or a Passkey. A session started with the password or a Sign-in provider alone does
not meet it for a User who has no Second factor; that session gets a 7-day grace period before access
is withheld.
_Avoid_: Enforced 2FA, mandatory MFA

**Second factor reset**:
An Owner or Admin removing another User's Second factor and Passkeys so they can re-enroll after losing both
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
The named set of Permissions a Membership holds in a Workspace: owner, admin or member. A
Workspace always keeps at least one owner, and only a Role holding `owners:manage` (the owner)
grants the owner Role or changes or removes another owner's Membership. No code compares a Role to
decide access: it asks a Permission.
_Avoid_: Permission (a Role holds Permissions, it is not one), scope (scope is the API-token term)

**Permission**:
A named action in a Workspace that a Role allows, such as managing API tokens or deleting the
Workspace. It belongs to a Membership through its Role, never to an API token: a token's
counterpart is its scope.
_Avoid_: Ability, capability, right, scope

**Audit entry** (the Audit log is the Workspace's append-only sequence of them):
An immutable record that an **actor** changed a Workspace's configuration or access — who
(a User, an API token, a platform Staff member shown to the customer only as "sphericon staff", or
`system`), what (an action on a target), and when. It keeps a snapshot of the target's name (a Contact by id only), so
it stays readable after the target is deleted; secrets are never recorded as values, and
Contact changes name the fields changed, never their personal data. It covers the
_control plane_ only (membership and roles, credentials, domains, integrations, webhooks,
content and audience definitions, Contact edits and deletes, imports, exports, logins) — never
the data plane, whose facts are already Events and Outbound messages. Readable by owner and
admin. An Enterprise concept, absent from a plain self-hosted install.
_Avoid_: Event (that is a Contact/data-plane fact), activity, history, changelog

**Setup checklist**:
The Workspace's path to its first sent email, shown on the Overview until it is done: a real Integration (a Test inbox does not count), a verified Sending domain, a Contact (any one, from any source), a sent email (an Outbound message
with a sent outcome, from any surface). Each step is
read off the Workspace's own data, never ticked by hand, so it cannot disagree with the Workspace;
a Sending domain still waiting for DNS is a step in progress and blocks no later step. A Test send
does not complete the last step.
_Avoid_: Onboarding wizard, getting started, tour

**Sign-up survey**:
An optional set of four one-question screens (role, company size, main goal, how the User heard of the
product) that a User who has just signed up, by password or through a Sign-in provider, answers once
after the first sign-in and before the dashboard. An Instance turns it on explicitly and a Staff
member reads the answers; they stay in the Instance's database and are never part of the Telemetry
report. A User who accepts an Invitation, the first Owner made by the CLI and Users that existed
when the survey was turned on are not asked. It completes nothing and personalizes nothing, so it is
not the Setup checklist.
_Avoid_: Onboarding wizard, questionnaire

**Sign-up item**:
One thing an Instance asks or tells a person when they sign up, set by a Staff member for the market
the Instance serves: a **required acceptance** (a checkbox without which no User is created), an
**optional consent** (an unticked checkbox sign-up never depends on) or a **notice** (a link, nothing
to tick). Each has its own Legal document, and one may stand for consent to the operator's mailings.
An Instance has none until a Staff member adds them: the core ships no legal text and knows no law.
An item no longer asked is archived (a **Discard**), never deleted.
_Avoid_: Consent (for the item), checkbox, terms (as the general name), retire

**Legal document**:
The text of a Sign-up item, kept in the Instance's database in the Instance's one language and shown
on a fixed public page of the app. Publishing makes an immutable version that takes effect at once; a
new version of a required acceptance stops every User who has not accepted it at an
accept-or-sign-out screen and refuses their Connections until they accept; the Workspace's API
tokens keep working.
_Avoid_: Policy page, terms URL

**Consent record**:
One decision of a User on one version of a Sign-up item's Legal document (accepted, declined or
withdrawn, or shown for a notice), with when and where it was made (the sign-up form, the provider
screen, an Invitation, a re-acceptance or account settings) and the IP address and user agent it
came from. Records
are only added, never changed; the latest per item is the User's current answer. It belongs to the
User, not a Workspace, and is unrelated to a Contact's consent proof.
_Avoid_: Acceptance flag, opt-in (alone)

### Usage & holds

The core _measures_ what each Workspace does and lets a Staff member hold sending from outside; it
keeps no allowance of its own. An external service reaches both through the Staff surface. A
self-hosted install has no such service and none of this is visible to it.

**Usage period**:
The calendar month, in UTC, over which usage is measured and a Usage snapshot is closed. The same
for every Workspace and owned by the core: the core closes a period before any reader treats its
figures as final.
_Avoid_: Cycle, anniversary period

**Usage snapshot**:
A per-(Workspace, Usage period, metric) materialized aggregate of activity, finalized (made
immutable) at period close. Distinct from the raw Events it is computed from, which stay the
source of truth for audit and dispute. Two metrics, aggregated differently: `emails_sent` is a
_sum_ of the Outbound messages the provider accepted over all three send surfaces, one per recipient,
in a single total, each counted in the Usage period it was accepted in (a suppressed, held or failed
message is never accepted, so never counted); `contacts` is a **high-water-mark**
(the peak reached during the period) of identified Contacts — those with an email Destination,
erased Contacts excluded — never an instantaneous value, and never anonymous tracker Contacts. It
materializes live Events the same way a Broadcast recipient's rollup materializes engagement —
the Events are truth, the snapshot is the closed, reproducible figure.
_Avoid_: Meter, counter, quota (quota is an allowance, not a measurement)

**Service credential**:
A machine identity of the Staff surface, next to the human Staff member and not a User: a bearer value
that lets a service call the service endpoints without a login, held by no Workspace and no
Membership. It carries a name, a set of scopes (read usage, read Workspaces, read Users, write holds) and an
optional expiry, and records when it was last used. Created, rotated and revoked only by the
`sphericon service-credential` command (no endpoint can mint or change one); the value is shown once
and only a hash is stored. A service may hold two active credentials at once so a rotation has no
downtime. The service endpoints are served only by a separate internal listener, never on the public
port. Its writes are recorded in the Audit log under its name, which customers see only as
"sphericon staff".
_Avoid_: API key, API token (that is a Workspace's), service account, Staff member

**Live usage**:
The same per-(Workspace, metric) figures for the Usage period still open: not final, with the
time it was last computed. Read from a worker or cache, never a recount on read, and counting
sends still in flight. It is what lets a reader notice a limit being approached before the period
closes; the Usage snapshot remains the only final figure.
_Avoid_: Current usage, real-time usage

**Staff hold**:
A reversible Workspace state that **freezes all outbound sending** — all three send surfaces
(Broadcast, Automation, Transactional), exactly like Workspace suspension — set by a Staff member
through the Staff surface for a reason other than reputation. Several can coexist: each is
stored separately under its own key, with a reason shown to the Workspace Owner, an optional action URL (where an Owner or Admin can resolve it) and an expiry the
setter must renew (required, and no further ahead than a configured maximum, 30 days by default), and each is cleared by its own setter or by its expiry. It is a **distinct
cause on the same core chokepoint**, never a repurposing of suspension: the send path asks one
question ("may this Workspace send now?") answered by several independent freeze reasons.
Messages are **held, not failed**: they wait and go out when the hold lifts. Every change records
the Staff member that made it.
_Avoid_: Suspension (that is the reputation freeze), ban, lockout

**Account link**:
A link in the app, shown only to a Workspace Owner and only when the installation is configured
with an address and a signing key, that opens an external account page carrying a short-lived
(about two minutes) signed token naming the user, the Workspace and the role. The token is signed
asymmetrically and the app publishes its public key as a key set, so the receiver verifies it with
no shared secret. The app never calls the address and keeps nothing about the receiving side.
Unconfigured, the operation answers 404 and the link is absent.
_Avoid_: SSO (nothing is federated: the receiver keeps no session of the app), magic link

### Self-hosting lifecycle

**Instance**:
One installation of the app: one database and every Workspace in it, whatever the number of
processes serving it. The unit that is upgraded, backed up and counted in a Telemetry report. The
SaaS is an Instance too, but never reports on itself.
_Avoid_: Installation, deployment, tenant (that is a Workspace), server

**Mode**:
Which kind of deployment an Instance is: `selfhosted` (the default) or `saas`. It alone decides
what differs between them, such as whether the Instance reports to the project; no separate
setting does. Sign-up is always open, whatever the Mode. It is not a License key and
unlocks no Enterprise feature.
_Avoid_: Edition (that is core or Enterprise, by License key), plan, tier, flavour

**Instance id**:
A random identifier an Instance generates for itself at first start. It is derived from nothing
(no Workspace, User, host or address) and identifies no one, only lets reports from the same
Instance be told apart over time.
_Avoid_: Install id, machine id, license id

**License key**:
The signed token that switches an Instance's Enterprise features on, with an optional expiry. With
none, or after it has expired, the Instance runs the core alone and loses no data; a key that does
not verify is a mistake and stops the start.
_Avoid_: Licence, serial, activation code

**Release check**:
The Instance asking where the newest published release is, so the owners of every Workspace and
the Staff member can be told one exists. It sends nothing about the Instance and is independent of
the Telemetry report.
_Avoid_: Update ping, phone-home

**Telemetry report**:
The anonymous, exactly documented summary an Instance sends to the project: coarse facts about the
Instance, never about a Workspace, Contact or person. On by default, switchable off by the
Self-hoster's configuration (which then wins) or by a Staff member in the Staff console. The first
one is sent only after a Staff member has acknowledged its notice there. Not the Self-hoster's own
OpenTelemetry metrics, which go to a collector the Self-hoster chooses.
_Avoid_: Usage ping, phone-home, analytics, tracking (that is the tracker on customer sites)

**Self-hoster**:
The person or team that runs an Instance on their own infrastructure. The app knows them as its
Staff member, once one is created; a Workspace's owner may or may not be the Self-hoster.
_Avoid_: Operator (the former name of a platform Staff member), system administrator, sysadmin,
admin (that is a Workspace Role)

**Upgrade**:
Moving an Instance to a newer release: the new version of the app and the database changes it
needs, applied before the new version serves anything. An Upgrade never stops an Instance that is
running.
_Avoid_: Update, migration (that is only the database part), deploy

**Major**:
A release that may remove what earlier releases added: a table, a column, a field of a queued job or
event. Every release between two Majors only adds. An Upgrade may skip any releases inside a Major
but passes through the last release of each Major it crosses; before 1.0 each `0.MINOR` is a Major.
_Avoid_: Breaking release

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
