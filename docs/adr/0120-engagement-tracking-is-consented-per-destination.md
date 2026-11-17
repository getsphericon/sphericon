---
status: accepted
build: none
---

# Engagement tracking is consented per Destination, and an untracked message leaves the rates

> **Amended by [[0123-engagement-history-is-unknown-for-an-untracked-destination]]:** a Condition's
> count of engagement is unknown for a Destination that is not tracked, so refusing tracking never
> reads as never opening.
>
> **Amended by [[0122-an-automation-branches-into-nested-paths-that-rejoin]]:** a Branch's test of an
> untracked message is unknown, and the Branch must name the path unknown takes.
>
> **Amended by [[0121-a-preference-page-per-destination-and-only-the-person-lifts-a-refusal]]:** the
> tracking page and its separate footer link are the Preference page, and a standing `denied` gives
> way only to the person's own grant there, no longer to a member's.

Every Broadcast and Automation email carries an open pixel and rewritten click links, and nobody
can refuse them: not the recipient, not the Workspace. An SMS Broadcast may keep its links as
written, but that is the author's switch, not the person's (ADR 0083). The pixel reads the
recipient's device, so ePrivacy art. 5(3) applies to it as it does to the tracker's cookie
(ADR 0093). The CNIL's recommendation on tracking pixels in email (deliberation 2026-042 of
12 March 2026) makes the recipient's consent the rule, and after a withdrawal the sender must stop
honouring the pixels of mail already sent. Rewritten links are not covered by it directly, but the
same consent rule of art. 82 of the French data protection act applies to them. A Workspace that
writes to French recipients has no way to comply today.

The consent the tracker waits for (ADR 0093) is about a browser on the customer's site; it says
nothing about a message in a mailbox. The two differ in subject (a Visitor, a Destination), in who
sets them (the site's banner; the person, or the customer who collected their consent) and in
lifetime, so they are two concepts. The one ADR 0093 introduced is renamed **Tracker consent**,
after the component it governs; "storage" read as data retention.

## Decision

- **Engagement tracking consent** is a fact per (Workspace, Channel, Destination), `granted` or
  `denied`, covering opens and clicks together: the open pixel, rewritten email links and Short
  tracked links. Like a Confirmation, its truth is an evidentiary System event (ADR 0019),
  `<channel>.engagement_tracking_granted` or `<channel>.engagement_tracking_denied`, with a derived
  row per Destination that the send path reads. It exists on every Channel that tracks
  engagement: email, SMS and Telegram today, and WhatsApp and web push once they send their tracked
  links (ADR 0083).
- **The Workspace's Engagement tracking default** (`track` or `dont_track`) decides for a
  Destination with no fact. It is `track` until the Workspace changes it; the core picks no default
  by country, as it ships no legal text (ADR 0062).
- **The decision is made at render and stamped on the Outbound message** as `tracked`: whether it
  went out with the pixel or its links tracked, a fact that never changes. The surface must track and
  the Destination consent: an SMS whose author kept its links as written goes out untracked too. An
  untracked message has no pixel and keeps its links as written; its unsubscribe and confirmation
  links and the `List-Unsubscribe` header stay, since they are not tracking. A retry renders tracked
  only when the attempt before it did and the message's tracking was not withdrawn since. A
  Transactional send's `track` (ADR 0053) is a request, not an override: consent wins.
- **Tracking withdrawn is a second fact of the message**, recorded once and never cleared, when a
  refusal or a default reaches it (below). A message counts as tracked while it went out tracked and
  its tracking was not withdrawn; nothing gives a withdrawn message its tracking back, neither a
  later `granted` nor a default turned back to `track`, so a message whose opens were deleted never
  returns to the rates as one that was never opened.
- **It is checked again when an open or a click arrives**, against the message's withdrawal and
  the effective consent now of the Destination the message went to, which the recorded Event names,
  not the Contact's current address. If the message's tracking was withdrawn or the Destination's
  effective consent is no longer to track (a `denied` recorded since, or a default turned to
  `dont_track` whose withdrawal has not reached the message yet), nothing is recorded: no Event, no
  stamp. The answer does not change: the pixel is served and the link redirects. A message whose
  Destination was erased resolves to no consent and records nothing.
- **A refusal reaches back.** When a `denied` is recorded, the tracking of the Destination's earlier
  messages is withdrawn and the opens and clicks recorded of them are deleted, their Events and
  their stamps alike, and so is the credit an Order took from such a click (ADR 0083): what was
  collected under a consent now withdrawn is not kept, and those messages leave the rates with their
  engagement. The CNIL allows aggregate figures, but a stamp on a message is not one. What was
  already done on them stays: an Automation the Event triggered, a path a Branch took, a Winner it
  picked, a message sent since. An Event kept on an Enrollment as `awaited.<name>` (ADR 0124) is
  deleted with the rest, and a later Step reads it as absent.
- **A default turned to `dont_track` withdraws, but deletes nothing.** The tracking of every message
  already sent to a Destination without a `granted` is withdrawn, in the background, and the
  messages rendered from then on go out untracked, so they leave the rates; what was recorded before
  stays: whether it was collected lawfully is the Workspace's question, and the Workspace made the
  change. A default turned back to `track` applies to the messages rendered from then on.
- **Who sets it, by provenance:**
  - `page`: the person, on a public page reached from a message to that Destination;
  - `manual`: a member of the Workspace, on the Contact card;
  - `imported`: the customer, through the API or an import;
  - `onsite_form`: an optional checkbox of an On-site form, for the email typed into it.

  A `denied` applies from any source. A standing `denied` is replaced only by a `granted` from
  `page` or `manual`: only the person, or a member acting on their request, undoes a refusal, never
  an import or a form. A `granted` from `imported` or `onsite_form` applies where nothing is
  recorded or a `granted` stands.

- **A form's grant has the standing of the email consent it comes with.** Under single opt-in it is
  recorded with the Form submit; under require-confirmed-opt-in it is carried by the confirmation
  link and recorded when the person confirms. Consent typed by someone else is none (ADR 0088), and
  a grant is no stronger than the address it rides with.
- **The page** is reached by a signed link that names the Channel, the Destination and the
  Workspace, like an unsubscribe link, so it works after the Contact is gone. `GET` shows the
  current choice and changes nothing; `POST` changes it (ADR 0012's split). It changes only that
  Destination on that Channel. The footer of every Broadcast and Automation email carries the link
  beside the unsubscribe link; SMS, Telegram and Transactional messages carry it only where the
  author puts its variable. It is not a page of every Sending source. When no Contact holds the
  Destination any more, the page offers only a refusal: a grant would keep the address of a person
  who was erased, for nothing they asked.
- **Erasure keeps a `denied`**, as it keeps an Unsubscribe (on SMS by the number's hash), so a
  re-imported address is not tracked again by the default. A `granted` goes, as a Confirmation does:
  losing it falls back to the default.
- **Engagement rates count tracked messages only.** The cohort's denominator is the tracked Broadcast
  and Automation messages of the window, and the reports show the untracked ones beside it, so a
  growing share of refusals does not read as falling engagement. The Complaint and Bounce rates do
  not change: delivery is not tracking.
- **Conditions reach it**: a field per Channel with `granted`, `denied` and not set, read live as the
  SMS reachability field reads Send-eligibility. Its Events trigger like any System event.

## Consequences

- A migration adds the per-Destination row, the Workspace setting, and `tracked` and the time its
  tracking was withdrawn on `outbound_messages` (add-only, ADR 0033). The email renderer's `Track`
  becomes "the surface tracks and the Destination consents"; SMS and Telegram set their
  `NoLinkTracking` from the same answer, which Telegram's renderer has to learn.
- Its observations are ordered as every other consent fact's (ADR 0154), on the Destination's
  `everything` stamp, as a Confirmation's are: it has no Sending source.
- Engagement recording (`outboundmessages/engagement`) resolves the message's Destination and its
  effective consent before it accepts an observation, so ADR 0113's projections never see a refused
  one. A `denied` removes the opens and clicks ADR 0113 keeps on the messages of that Destination,
  which that ADR keeps through Erasure: a withdrawal is not an Erasure, and it takes them.
- An Automation trigger on `email.opened` or `email.clicked` never fires for an untracked message. For
  a future branch on "opened", an untracked message's open is unknown, not absent: the branch must
  route it on purpose.
- Revenue attribution by click (ADR 0083) has nothing to credit for a Destination that refused, and
  an Order credited from one of its clicks before the refusal loses that credit.
- The API and the import gain the field with its provenance; the On-site form editor gains the
  checkbox; the confirmation link carries the form's grant.

## Considered options

- **One `tracked` stamp, cleared when tracking is withdrawn.** It would forget what went out, and a
  stamp a later grant could set again would bring back to the rates a message whose opens were
  deleted, as one never opened.
- **A default turned to `dont_track` that reaches only the messages rendered after it.** The
  messages already in mailboxes would stop recording and stay in the cohort, so the rates would fall
  for no change in the mail.

- **Two attributes on the Contact, opens and clicks, each with a source and a date** (Notifuse does
  this). A Contact attribute does not survive Erasure and re-import, cannot differ between two
  addresses of one Contact, and departs from how every other consent fact here is kept.
- **Opens and clicks consented separately.** People refuse being tracked, not a mechanism; a second
  switch doubles the rows, the Events and the page for a choice almost nobody makes.
- **Untracked messages left in the denominator.** The rates would fall with every refusal and say
  nothing about the mail.
- **A default per Instance or per country.** The core knows no law; the Workspace knows whom it
  writes to.
- **Checking consent only at render.** A withdrawal would leave every message already in the
  mailbox reporting opens.
- **A form's grant recorded at submit whatever the opt-in policy.** Under require-confirmed-opt-in it
  would make tracking consent the one thing a stranger can give for an address.
- **Keeping what was recorded before a withdrawal.** The figures of past windows would stay put,
  but the opens of a person who refused would be kept with no consent behind them.
- **A deliverability-only mode now.** The CNIL exempts recording the date of a recipient's last open
  to prune inactive recipients of mail they asked for. It is a separate feature with its own limits
  (no per-message Events, no triggers) and is left for later.
