---
status: accepted
---

# Send cap and Provider limit across Channels

ADR 0067 set two limits: the Provider limit, per Integration, mirroring the customer's provider
account (per second and per rolling 24 hours, the lower of the user's value and the provider's);
and the Send cap, per Workspace on a `saas` Instance, set by Staff against abuse, a Deferral when
spent. Both were written for email. The research on the map issue (getsphericon/sphericon#916)
found a Telegram bot limited to about 30 messages a second by a documented constant, WhatsApp
limits readable through the API, and SMS limits that vary by provider and are rarely readable.

## Decisions

- **One Send cap counts every Channel.** Each message counts once, whatever its Channel. The cap
  protects the platform from abuse, and spam by SMS or messenger harms it as much as spam by email;
  one cap per Channel would give Staff six numbers to keep by hand.
- **The Provider limit works the same on every Channel,** with these starting values:
  - Telegram: 30 a second, filled in when the bot is connected; the user may lower it.
  - WhatsApp: read from the provider's API like SES's quota, the phone number's throughput as the
    per-second value and the messaging limit as the 24-hour value. Meta counts unique people, not
    messages, which a send of one message per Contact makes nearly the same.
  - SMS: the user's value only, optional.
  - Web push and mobile push: none by default.
- **Warmup stays email's.** Sender reputation warmup is an email notion; its SMS counterpart
  (10DLC registration and carrier tiers) lives at the provider, as it does for messengers.
