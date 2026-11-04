---
status: accepted
---

# The tracker waits for storage consent when asked, and forms show without it

The tracker writes its Visitor cookie (`om_vid`, 365 days) and a localStorage queue on `init()`, with
no way for a site's consent banner to hold it back. ePrivacy art. 5(3) makes any storing in, or
reading back from, the visitor's device need consent unless it is strictly necessary for a service
the visitor asked for, whether or not it is personal data (Planet49; EDPB Guidelines 2/2023). WP29
Opinion 04/2012 exempts customisation the user explicitly requested, by a click, only for the
session or a short term. Showing a form stores nothing. Klaviyo splits its tracking scripts from its
forms so that a consent manager can block one and keep the other.
The legal targets are the US and the EU. Decided in the
frequency ticket of the on-site map (getsphericon/sphericon#957).

## Decisions

- **The tracker has a Storage consent state:** `granted`, `pending` or `denied`, given in `init` or
  changed by a call from the site or its consent manager. `granted` is the default, so a site with
  no banner works as today.
- **The Workspace can make waiting the default.** A setting on the Tracking page puts `pending` into
  the snippet it hands out, so an EU customer protects visitors by default with one switch
  (GDPR art. 25) and grants from the banner's callback.
- **Until consent is granted the tracker stores nothing and sends nothing about the visitor:** no
  Visitor cookie, no queue, no Events; a `denied` state also clears what an earlier grant stored.
- **On-site forms still load and show.** A close is remembered only for the session, as the
  visitor's own request; a frequency rule the author set is kept only after a grant. A Form submit
  still works, as something the visitor sends on purpose, but binds no Visitor (ADR 0086) since there
  is none.
- **Adapters for consent managers come later.** Google Consent Mode v2, IAB TCF v2, Cookiebot and
  OneTrust each expose an API the tracker could read; until then the site calls the tracker from
  its banner's callback.
- **The consent call and the `init` option are a contract** customer sites carry (ADR 0036).
