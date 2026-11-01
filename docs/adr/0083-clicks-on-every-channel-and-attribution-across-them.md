---
status: accepted
---

# Clicks on every Channel, and attribution across them

Email links go through the Tracking domain and record `email.clicked`; a click by software is
machine engagement and counts nothing (ADR 0053); revenue goes to the Broadcast or Automation of the
Contact's last `email.clicked` inside the Attribution window (ADR 0064). The new Channels carry links
too, and the research on the map issue (getsphericon/sphericon#916) found every commercial product
tracks SMS and WhatsApp links through a short domain, with a click event per Channel.

## Decisions

- **sphericon rewrites the links it renders,** through the Tracking domain or the Instance's host,
  as short tokens (`/c/<token>`), never a third-party shortener: the SMS text, Telegram's text and
  link buttons, web push's click link, and on WhatsApp the variable values and link-button suffixes
  that are links. Links fixed in a WhatsApp template's text stay untracked, as Meta holds them. An
  SMS may switch tracking off to save its segments; it is on by default.
- **A click per Channel.** `sms.clicked`, `whatsapp.clicked`, `telegram.clicked` and
  `web_push.clicked`, each carrying its Broadcast or Automation as `email.clicked` does. A
  messenger fetching a link for its preview is machine engagement; Telegram previews stay on unless
  the author turns them off, since they help the message and the fetch is filtered anyway.
- **Mobile push clicks come from the push service.** A tap opens the app, often by a deep link that
  cannot pass through a redirect, so the service's own click event, read by its Preset, records
  `mobile_push.clicked`. A Delivery notice gains the kind `clicked` for it, which extends the closed
  set of ADR 0076.
- **Attribution counts a click on any Channel.** An Order is credited to the Broadcast or Automation
  of the Contact's last human click on any Channel inside the Attribution window; one window serves
  every Channel. A WhatsApp read, a push delivery or an email open credits nothing. Separate windows
  per Channel were not taken: one number a Workspace can reason about beats a finer split nobody
  tunes.
