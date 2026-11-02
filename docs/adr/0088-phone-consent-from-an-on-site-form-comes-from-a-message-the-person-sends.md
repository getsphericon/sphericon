---
status: accepted
---

# Phone consent from an On-site form comes from a message the person sends

An On-site form (ADR 0085) posts through the tracker's public collect key, so a submit proves only
that some browser typed an address or a number (`research/browser-form-consent`). In the US the
seller carries the whole burden of proving consent, and consent typed by someone else is none; in
Germany double opt-in documents an email consent and proves nothing about a phone, and the DSK
rates SMS codes as weak evidence. The legal targets are the US and the EU. While charting, the map
(getsphericon/sphericon#948) allowed reopening ADR 0077 so that a form could check a phone with a
code sent through the customer's SMS Integration. Decided in the consent ticket
(getsphericon/sphericon#954).

## Decisions

- **No codes.** A form that makes the Instance send an SMS to any number typed in is the textbook
  SMS-pumping target (OWASP OAT-003), and Twilio, AWS and Braze bill the sender: the customer, and on
  a self-hosted Instance only the customer. A code is also a message before consent, which ADR 0077
  rules out. ADR 0077 stands: sphericon runs no SMS double opt-in of its own.
- **The phone writes first.** A form's SMS or WhatsApp element opens an `sms:` or `wa.me` link
  prefilled with `START <token>` on a phone, and shows the same as a QR code on a desktop, as
  Klaviyo's tap-to-text and Attentive's Two-Tap do. The token is a Start token (ADR 0076) issued to
  the current Visitor. Nothing is sent to the number before the person writes. The START records a
  Confirmation with the existing provenance `keyword` and lifts an opt-out, as ADR 0077 already has
  it; a provider that recognises START with trailing text also lifts its own block.
- **A valid token makes the sender known.** ADR 0076 creates nothing for an unknown sender; a token
  sphericon issued is the exception. The Confirmation is recorded on the (Channel, number) in any
  case. If the Visitor's Contact has no phone and no other Contact holds the number, the number is
  given to it; if another Contact holds it, the message is that Contact's and the Visitor stays
  where it is (ADR 0086); if the Visitor is anonymous, the Contact is found by the number or created
  with the source `onsite_form`, and the Visitor is bound to it.
- **A phone field gives no consent.** A number typed into a form is only a Contact attribute (ADR
  0086).
- **The element names its Integration,** the Channel's default unless the author picks another; it
  must receive inbound messages. Publish is refused (409, as #787 amended ADR 0065) while that
  Integration is disabled, archived or receives no inbound messages. A customer whose provider
  cannot deliver inbound messages collects no SMS or WhatsApp consent through forms.
- **The confirmation text after opt-in** that CTIA asks for is the provider's START auto-reply where
  it has one (Twilio's Advanced Opt-Out); otherwise the customer's welcome Automation triggered by
  `marketing.confirmed` sends it, as a marketing message after consent. sphericon still sends no
  compliance replies (ADR 0077).
- **The exact disclosure is kept.** A form's `marketing.confirmed` Event carries the form, its
  version and the disclosure text the person saw (CTIA, 10DLC, GDPR art. 7). The author writes it
  beside the button, from a default naming the brand, the frequency, carrier rates, STOP and HELP
  and a terms link, never bundled with the email consent.
- **Email follows the Workspace.** With require-confirmed-opt-in on, a form sends the double opt-in
  email (ADR 0013); off, the address is subscribed once, as today. A form submit never lifts an
  `everything` Unsubscribe or a Suppression: returning through a form always takes the confirmation
  email, whatever the policy.
- **Web push and Telegram need nothing new:** the push button calls `subscribeToPush()` on the
  click, the Telegram button opens `t.me` with the Visitor's Start token, and the Binding is the
  opt-in (ADR 0075, 0076).
- **Bots are slowed by proof-of-work.** Every Form submit carries an invisible proof-of-work
  solution (ALTCHA-style, verified in Go) beside the existing per-Workspace and per-IP collect
  limits. Hosted captchas need an account and registered hostnames, which a self-hosted Instance
  does not have. The challenge is part of the collect path, a contract under ADR 0036.

## Consequences

- Whether a `wa.me` message the person sends counts as a WhatsApp marketing opt-in is not stated
  by Meta (Klaviyo treats it as one); it is the best option found, not a guarantee. The customer's
  disclosure and carrier registration need their own counsel.
- Whether Twilio and other providers treat `START <token>` as START is checked when the SMS
  element is specified.
