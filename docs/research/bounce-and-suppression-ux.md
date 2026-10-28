# Bounce and suppression UX: where vendors surface them, and the soft-bounce rule

Researched October 2026 from primary sources only: the vendors' own help centers and developer docs (Klaviyo, Brevo,
Mailchimp, Customer.io, SendGrid, Postmark, Loops, Mindbox) and the listmonk source, shallow-cloned at
`a74f40b346aca27ceed3e2bbf6528712d3d16375` (default branch, October 2026). No third-party blog is cited. Where a vendor
page could not be read, the claim is moved to "Unconfirmed" at the end. sphericon's side is read from `GLOSSARY.md`
(Suppression, Delivery notice) and the ADRs named in the question; for Mindbox as a product see
[mindbox-feature-analysis.md](mindbox-feature-analysis.md), which is not repeated here.

## Question

How do email and marketing-automation products show bounces and suppressions to the workspace user, and what do they
do with repeated soft bounces? Four surfaces matter: a top-level bounces page, a per-send "who bounced" list, an
automatic suppression after N soft bounces, and the place of the Suppression list in navigation.

## Summary of findings

- **Per-send drill-down is the norm, a top-level Bounces page is rare.** Klaviyo, Mailchimp and listmonk list the
  bounced addresses from the campaign report. Only listmonk (and SendGrid, an API-first sender) has a Bounces entry
  in the main navigation.
- **The suppression list is a settings or audience-level surface**, not a reporting one: Customer.io under
  workspace settings (admin only), Klaviyo on the Profiles page, Postmark per message stream, SendGrid under
  Suppressions.
- **Hard bounce and complaint suppress at once everywhere checked. Soft bounce handling varies:** a count threshold
  (Klaviyo 7 consecutive, Mailchimp 7 or 15, Mindbox 3 in 7 days, listmonk configurable) or no automatic action
  (Postmark, Customer.io, SendGrid as documented).
- **The threshold semantics differ.** Klaviyo counts consecutive soft bounces and a delivery resets the streak.
  Mailchimp varies the limit with prior engagement. Mindbox requires no opens or clicks in the window. listmonk counts
  all bounces of a type for the subscriber, not consecutive ones.

## 1. Surfaces per vendor

### Klaviyo

- Suppressed profiles: Audience > Profiles > "View suppressed profiles" for global email suppressions; list-level
  suppressions are under the list's Manage list menu. Source: [Understanding suppressed email profiles](https://help.klaviyo.com/hc/en-us/articles/115005246108).
- Per-send drill-down: the sent campaign's Recipient activity tab has a Bounced section, where each bounce shows
  category, specific reason and type (hard or soft). Source: [Understanding bounced emails in Klaviyo](https://help.klaviyo.com/hc/en-us/articles/115005250408).
- Hard bounce: the address is suppressed automatically. Source: same article.
- Soft bounce: suppressed after 7 consecutive soft bounces within the past 2 years, and a successful delivery resets
  the streak. The article says "7 consecutive times". Source: same article.
- Manual path: the article suggests a "chronic soft bounce" segment so an admin can suppress earlier than 7, and
  temporary suppression for soft bounces classed "Unclassified" or "Mailbox unavailable". Source: same article.

### Mailchimp

- Per-send drill-down: Analytics > Reports > the campaign > click the Bounced number, then "Bounce Reason" per
  address. Bounce reasons are stored for 30 days and cannot be exported as a full list. Source: [View Bounce Reasons](https://mailchimp.com/help/view-bounce-reasons/).
- Rule: 7 soft bounces allowed for an address with no subscriber activity, up to 15 for contacts with prior
  activity, after which the soft bounce is converted into a hard bounce. Hard bounces are cleaned "automatically and
  immediately". Source: [Soft vs. Hard Bounces](https://mailchimp.com/help/soft-vs-hard-bounces/).
- Audience surface: "Cleaned" is a subscription status on the contact. The help page reaches the contacts through a
  segment with Email subscription status is one of Cleaned (no dedicated menu entry is described). Cleaned means hard
  bounced or repeatedly soft bounced, and cleaned addresses cannot receive future audience sends. Source: [About Cleaned Contacts](https://mailchimp.com/help/about-cleaned-contacts/).

### Customer.io

- Suppression list: Workspace Settings > Email > Suppression List, visible to Account Admins and Workspace Admins
  only, and only with Customer.io-managed delivery (with Custom SMTP the list lives in the ESP). The API names four
  reasons: `bounces`, `blocks`, `spam_reports`, `invalid_emails`. Source: [ESP suppression lists](https://docs.customer.io/journeys/esp-suppression/).
- A second page describes "Workspace Settings > Workspace Suppressions" holding two kinds: ESP-sourced email
  suppressions (hard bounce, spam complaint, ESP Suppression API) and GDPR suppressions of a person. An
  ESP-suppressed address blocks email only, the person can still receive other channels and enter automations.
  Source: [How to manage suppressions](https://docs.customer.io/messaging/channels/email/deliverability/workspace-suppressions/).
  The two pages give different menu paths; the product UI may have moved between them.
- Per-message: a bounce's reason and status code show in Message activity or the person's Activity tab. Message
  activity does not label a bounce hard or soft. Segments can use a "has ever bounced" condition filtered by reason
  text. Source: search excerpt of [Message statuses](https://docs.customer.io/messaging/channels/message-statuses/) and the
  deliverability pages (see Unconfirmed; the page itself was not read).

### SendGrid (Twilio)

- Console: Suppressions > Bounces, filterable by date and exportable as CSV, with delete for single or all rows.
  Source: [Bounces](https://www.twilio.com/docs/sendgrid/ui/sending-email/bounces).
- Suppression kinds: Block (not kept suppressed), Bounce, Invalid Email, Spam Report, Global Unsubscribe, Group
  Unsubscribe (all kept). Source: [Suppressions](https://www.twilio.com/docs/sendgrid/ui/sending-email/index-suppressions).
- Event model: a `deferred` event is a temporary rejection with an `attempt` counter; `bounce` is hard
  (permanent); `blocked` is the soft bounce, temporary, and not auto-suppressed per the table above. The event also
  carries `bounce_classification` (Invalid Address, Technical, Content, Reputation, Frequency/Volume, Mailbox
  Unavailable, Unclassified). Source: [Event webhook](https://www.twilio.com/docs/sendgrid/for-developers/tracking-events/event).

### Postmark

- Per-stream Suppressions tab lists inactive addresses with a reason; there are four suppression types: hard bounce,
  unsubscribe, spam complaint, manually added. Hard bounces, unsubscribes and manual rows are reactivated with a
  button or the Suppressions API; spam complaints cannot be self-reactivated. Source: [An address has been marked as inactive](https://postmarkapp.com/support/article/903-an-address-has-been-marked-as-inactive-how-can-i-reactivate-it).
- Only hard bounce and spam complaint deactivate automatically; the article does not say soft bounces do. The
  Activity page, filtered by bounce type, opens each email's full SMTP response. Source: [What are bounces and spam complaints?](https://postmarkapp.com/support/article/what-are-bounces-and-spam-complaints).
- API model: 22 bounce types (HardBounce 1, Transient 2, SoftBounce 4096, SpamComplaint 100001, Blocked 100006,
  and others), each bounce carries `Inactive` (did this bounce deactivate the address) and `CanActivate`. Raw dumps
  are kept 30 days, bounces 45 days by default. Source: [Bounce API](https://postmarkapp.com/developer/api/bounce-api).

### Loops

- Hard bounce and complaint put the contact into suppression, which blocks all sends; soft bounces are retried and
  tracked separately. Source: [Reducing product email bounce rate](https://loops.so/docs/deliverability/reducing-bounce-rate)
  and [Contact suppression](https://loops.so/docs/contacts/suppression).
- The suppression page documents check and remove through the API only; it does not say where, if anywhere, a
  dashboard lists suppressed contacts. Source: same page.

### Mindbox

- Rule from the help center: a non-existent address is invalid from the first send; for "address does not accept
  mail" or unknown errors, three or more failures within 7 days with no clicks or opens make it invalid; a full or
  inactive mailbox is invalid only temporarily; blacklist, content-rejection and connection-drop errors never
  invalidate. The article does not use "soft" and "hard". Source: [Email delivery failure reasons (in Russian)](https://help.mindbox.ru/docs/email-not-delivered-reason).
- Report: the mailing report shows Bounce Rate and a count of undelivered messages; the page describes no list of
  bounced addresses. Source: [Mailing reports (in Russian)](https://help.mindbox.ru/docs/mailings-dashboard).
- Per-contact: an invalid-contact icon sits beside the email on the client card and a "mark as valid" action restores it.
  Source: [Restoring an email address from the invalid list (in Russian)](https://help.mindbox.ru/docs/email-validate-manually).

### listmonk (source code)

- Navigation: sidebar group Subscribers with All subscribers, Import and Bounces (`static/admin/partials/sidebar.html`,
  lines 22-37). A blocklisted subscriber is a subscriber status, not a separate list (see also
  [outbound-send-prior-art.md](outbound-send-prior-art.md)).
- Three entry points to the same table (`static/admin/partials/bounces-table.html`): the Bounces page, a Bounces tab on
  the subscriber (`views/subscriber.html` line 50), and a link from each campaign's bounce count to
  `/admin/subscribers/bounces?campaign_id=<id>` (`partials/campaigns-table.html` line 150). Columns: email, campaign,
  source, type (hard, soft, complaint), created at; bulk delete.
- Rule: Settings > Bounces holds per-type `{count, action}` with actions none, unsubscribe, blocklist, delete.
  Defaults in `schema.sql` line 290: soft `{2, none}`, hard `{1, blocklist}`, complaint `{1, blocklist}`.
- Semantics: `queries/bounces.sql` (`record-bounce`) counts all stored bounces of that type for the subscriber plus the
  new one and fires the action when the count is at least the threshold. It is a running total, not consecutive and
  with no time window, and no later delivery resets it. Classification: keyword heuristics on the bounce mail body, with
  "soft" as the fallback ([docs](https://listmonk.app/docs/bounces/)).

## 2. Cross-vendor patterns

| Question                      | What the sources show                                                                                                                                                                      |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Top-level Bounces menu        | listmonk (inside the Subscribers group); SendGrid Suppressions > Bounces. Not described for Klaviyo, Mailchimp, Customer.io, Postmark (stream tab) or Mindbox.                             |
| Per-send "who bounced"        | Klaviyo Recipient activity > Bounced; Mailchimp report > Bounced > Bounce Reason; listmonk campaign link. Mindbox report: counts only. Postmark: Activity filtered per stream.             |
| Suppression list placement    | Settings-level (Customer.io), audience/profiles-level (Klaviyo), channel-level (Postmark stream), compliance section (SendGrid). Always separate from the contact list.                    |
| Soft bounce auto-rule         | Klaviyo 7 consecutive (2-year window); Mailchimp 7 or 15 converting to hard; Mindbox 3 in 7 days without opens or clicks; listmonk configurable, off by default; Postmark none documented. |
| Reason detail kept per bounce | Klaviyo category and reason; Mailchimp reason (30-day retention); Postmark type and SMTP dump (30 days); SendGrid `bounce_classification`; Customer.io reason and status code.             |
| Lift action                   | Postmark and SendGrid and Customer.io (GDPR kind) remove; Postmark forbids self-lifting complaints; Mailchimp cleaned contacts cannot be edited, a corrected address is a new contact.     |

## Implications for sphericon

This section is analysis, not sourced findings. sphericon already stores `bouncedAt`, Delivery notices and per-domain rates.

- **(a) Dedicated "Bounces" menu group: not needed.** Only listmonk has one, and its table exists mostly because bounces
  are a separate record from subscriber status. A Bounces entry would duplicate the Suppression list for hard bounces
  and show nothing for soft ones until they are stored as rows.
- **(b) Drill-down inside the broadcast and automation report: yes, this is the common pattern.** Klaviyo, Mailchimp
  and listmonk all link the bounced count to a list of address, type and reason. sphericon already has the count and
  `bouncedAt` per outbound message, so the cheapest step is to make the count a link to the filtered messages with
  the provider reason from the Delivery notice. Mailchimp's 30-day limit on reasons is a reminder to keep the reason
  on the outbound message, not only in the provider.
- **(c) Auto-suppress after N consecutive soft bounces: optional, worth a fixed default rather than a setting.** Two vendors that document a rule use a count of about 7 (Klaviyo, with a reset on success; Mailchimp, with an engagement-dependent cap). Counting consecutive soft bounces per (channel, destination), reset by a delivery, is
  the safer semantics than listmonk's running total. A suppression reason distinct from `bounce` would keep the lift
  path and the report honest. Because GLOSSARY says bounce and complaint are a compliance ratchet that does not
  auto-clear, an auto-suppressed soft-bounce address probably wants to be liftable without that ratchet. Postmark and
  SendGrid show that not suppressing soft bounces at all is also accepted.
- **(d) Where the Suppression list belongs: next to Contacts, as an audience-level page, not under reporting.** Klaviyo
  puts it on the Profiles page and Postmark on a per-stream tab; Customer.io puts it in settings but restricts it to
  admins, which is an argument for putting the lift action behind a role, wherever it sits. A placement under
  Contacts (or Audience) fits sphericon's Suppression being destination-centric and including destinations with no
  contact; a Settings placement fits an admin-only view.

## Unconfirmed

- **Brevo, all of it.** help.brevo.com returned HTTP 403 to the fetcher. A search excerpt of the official article
  ([What are soft bounces and hard bounces in email?](https://help.brevo.com/hc/en-us/articles/209435165-What-are-soft-bounces-and-hard-bounces-in-email))
  says hard bounces are blocklisted at once, deferred mail is retried for about 36 hours, and five consecutive soft
  bounces on a campaign blocklist the address, not for transactional email. The page was not read. The claims that the
  bounce reason appears on the contact's detail page and that a segment condition "classed as hard/soft bounce"
  exists were not found at all.
- **Customer.io message-level claims** (retries up to 8 times over 12 hours, "Too old" status, mailbox full not
  suppressed, "has ever bounced" segment condition) come from a search-tool summary of docs.customer.io, not from
  reading the pages. The suppression-list access rule and types were read directly (see section 1).
- **Klaviyo "more than 7" versus "7 consecutive".** The page read directly says 7 consecutive times; search excerpts
  of other Klaviyo pages say "more than 7". Treat the exact off-by-one as unresolved.
- **Mindbox "three soft bounces or one hard bounce".** Appears only in a Mindbox marketing case study
  (mindbox.ru/journal/cases/emkashop/), seen as a search excerpt. The help-center rule (3 failures in 7 days, no
  opens) is the one used above.
- **Mailchimp**: no help page read describes a menu for cleaned contacts beyond the segment route, or whether cleaned
  contacts can be resubscribed.
- **Loops**: whether the dashboard lists suppressed contacts is not stated on pages read.
- **Postmark**: whether a SoftBounce (4096) ever sets `Inactive` is not stated; the support article lists only hard
  bounce and spam complaint as automatic.
- **SendGrid**: the "72 hours" deferral window and where Suppressions sits in the console navigation were not found
  on the pages read.
- **listmonk**: read statically from `queries/bounces.sql` and `schema.sql`, not run.
