---
status: accepted
---

# Message content per Channel

Message content was email's subject plus HTML with Liquid merge tags (ADR 0040), and a Template is
a saved copy of it. The new Channels (ADR 0074) carry different things: a text on SMS, formatted
text with media and buttons on Telegram, a title and a link on push, and on WhatsApp only a message
Meta approved in advance. The multichannel research on the map issue (getsphericon/sphericon#916)
found no product that shares one body across channels.

## Decisions

- **Content is per Channel and never shared.** Email: subject and HTML, as now. SMS: a text.
  Telegram: text in the HTML subset Telegram accepts, an optional image, link buttons. Web push:
  title, text, the link opened on click and an optional image; the icon is the Integration's.
  Mobile push: title, text, a link or deep link and an optional image, handed to the push service
  as they are. WhatsApp: a WhatsApp template reference. Media are https URLs; sphericon stores no
  uploads for them.
- **One merge-tag language.** Liquid with the same bindings on every Channel.
- **Templates stay email-only.** On the other Channels the text is written in the Broadcast or the
  Automation step, where it is short enough that a library adds little; a Template with a Channel
  can be added later without reshaping anything.
- **WhatsApp content is a reference.** WhatsApp templates are created and reviewed in Meta's or the
  provider's tools. sphericon lists an Integration's approved ones through the provider's API; the
  author picks one and maps each numbered variable to a Liquid expression, plus a header media URL.
  The message keeps the name, the language and the mapping. The copy-at-author-time rule cannot hold
  because Meta keeps the text, and a template Meta pauses or disables makes the send fail (whether
  that holds the source is decided with the Outbound send). Creating templates from sphericon is
  not built.
- **sphericon appends nothing.** Email keeps its unsubscribe footer. On the other Channels an
  `{{ unsubscribe_url }}` merge tag is offered, and a Telegram message may carry an unsubscribe
  button (a Channel-wide opt-out, ADR 0077); the opt-out wording CTIA asks for is the author's.
  An automatic line would cost SMS segments and read differently per country.

## Consequences

- A Workspace that wants one announcement on email and SMS writes it twice.
- What a WhatsApp message says can change or vanish at Meta between authoring and sending.
