---
status: accepted
---

# A Broadcast has one Channel

A Broadcast was an email: subject, sender, HTML body, an audience Segment, an Integration and a
lifecycle with a planner, holds, deferrals, Recipients and derived figures. Each new Channel needs
one-off sends too. The multichannel research on the map issue (getsphericon/sphericon#916) found
most products keep one channel per broadcast object (Customer.io, Iterable, Brevo, Mautic,
Dittofeed); Braze and Klaviyo group several channel messages under one campaign.

## Decisions

- **One Broadcast entity with a Channel, fixed at creation.** Its Message content takes the
  Channel's shape (ADR 0078); its lifecycle, Segment and Condition copy, Integration, planner,
  holds and deferrals, Broadcast recipients and figures are shared by every Channel. A separate
  entity per Channel was rejected: it would repeat the hardest parts six times to vary the one
  part, the content, that a per-Channel editor already varies. A multi-channel campaign object
  (Braze, Klaviyo) was not wanted.
- **A Broadcast is never converted** to another Channel, nor copied into one: the content does not
  carry over.
- **The `broadcasts` Sending source needs nothing new.** Unsubscribes are keyed by Channel, so a
  `broadcasts` opt-out on email leaves SMS untouched; an opt-out made in a Channel is Channel-wide
  anyway (ADR 0077).
- **Figures per Channel, still derived.** Recipients, sent, skipped, failed, pending and clicked
  on every Channel; opened, bounced and complained on email; delivered and undelivered on SMS and
  WhatsApp, read on WhatsApp, delivered on mobile push when the service reports it (ADR 0079).
- **One list.** Broadcasts of every Channel share one list with a Channel column and filter; a new
  Broadcast asks for its Channel first, offering only Channels the Workspace has an Integration for.
- **Schedule also checks the WhatsApp template.** Beside the Send route, a WhatsApp Broadcast is
  refused at Schedule unless its template is approved; a pause after that holds it (ADR 0079).
- **Test send** on SMS and WhatsApp goes to a phone number the author types, through the
  Broadcast's Integration, skipping Send-eligibility as on email. Telegram and push have none: the
  author would need a Binding of their own.
