# Trustworthy consent from a browser form: bot protection, phone verification, law

Researched 2026-11-01 for #950 (map #948) from primary sources. These include:

- the FCC's own orders (docs.fcc.gov) and the eCFR;
- the CTIA Messaging Principles and the CTIA Short Code Monitoring Handbook;
- Twilio, AWS, Meta and the vendors' help centres and developer docs (Klaviyo, Attentive, Postscript, Braze, Mautic);
- the docs, RFCs and repositories of the bot-protection products;
- EU texts from the Publications Office (Cellar), EDPB guidelines and the WP29 opinion;
- the German BGH judgment and the DSK guidance;
- Russian law and FAS letters as КонсультантПлюс (`consultant.ru`) serves them.

A claim not confirmed in a primary source is marked **unverified**.

**It builds on, and does not repeat,** the channels map's consent research (#922, branch
`research/channel-consent-rules`, `docs/research/channel-consent-rules.md`, cited below as _CCR_). That note already
covers the rules below; this note adds what is new when the consent comes from a form in a browser.

- TCPA written consent (64.1200(f)(9)).
- The vacated one-to-one rule: removed as of 2025-04-30 (90 FR 42137), so it does not apply to a web form either.
- FCC 26-67 revocation.
- CTIA 5.1: the call-to-action, proof fields and opt-in confirmation.
- Meta's opt-in page.
- Russia: 38-FZ art. 18, 126-FZ art. 44.1 and 152-FZ.
- GDPR art. 7.

The prior capture research (`research/on-site-messages-and-forms`, `docs/research/on-site-messages-and-forms.md`)
covers the vendors' forms in general.

**Tags.** **[law]** marks a statute, regulation or court. **[policy]** marks a platform, carrier or industry rule. **[spec]**
marks a technical standard. **[vendor]** marks a product's documented behaviour.

**Limits.** Several sources could not be read in full:

- Attentive's Two-Tap and desktop compliance articles sit behind a login.
- The FAS and Russian court databases (fas.gov.ru, vsrf.ru) refused connections.
- altcha.org did not resolve, so only ALTCHA's GitHub repositories were read.
- Some Braze and Twilio pages were read only through a search summary; those claims are marked.

**This is research, not legal advice.**

Terms as in `GLOSSARY.md`: **Confirmation** (provenances `double_opt_in`, `grandfathered`, `imported`, `keyword`),
**Destination**, **Unsubscribe**, **Binding**, **On-site message** (being settled in #952). ADRs: 0013 (the
Confirmation), 0077 (consent on the new Channels: sphericon runs no SMS double opt-in of its own), 0084 (on-site is
built), 0036 (tracker paths are a long-lived contract).

## Question

The tracker's collect key is public. What makes a consent submitted from a browser form trustworthy enough to record
on each Channel? Five areas:

- bot protection, and what works on a self-hosted Instance with no third-party account;
- how vendors verify a phone typed into a popup, and what that costs;
- SMS pumping (toll fraud) against such a form;
- what the law and carriers require of a web-form SMS or WhatsApp opt-in;
- email double opt-in, and cookie consent for the popup itself.

## Summary

- **Anyone can submit anyone's Destination.** The form posts with a public key, so a submission proves only that _some
  browser_ typed a number or address.
  - **US [law]:** the FCC accepts a "website form" as written consent under E-SIGN. The seller bears the whole burden
    of proof, and consent typed in by a third party is no consent at all.
  - **Germany:** the BGH holds that email double opt-in plus a ticked box documents email consent but proves nothing
    about a phone [law]. The DSK calls double opt-in "geboten" and rates SMS codes as weak evidence [guidance].
  - **Russia [law]:** a form that does not identify the subscriber is inadequate.
- **The FAS letter of 11.11.2019 N ДФ/98054/19 is marked "утратил силу или отменен" on КонсультантПлюс.** CCR (4.1)
  and ADR 0077 ("Russia's FAS reading") relied on it for "a site form alone is not enough". No replacing letter was found. The rule
  still stands on two texts in force:
  - 126-FZ art. 44.1(1): an action "однозначно идентифицирующих этого абонента";
  - the ВАС Plenum N 58 para 15.
    No primary source was found that accepts an SMS code as that action.
- **Bot protection with no third-party account exists:** proof-of-work (ALTCHA, MIT, with a Go library; Cap,
  self-hosted), plus rate limits.
  - Turnstile, hCaptcha and reCAPTCHA all need an account, a secret and a server call to the vendor.
  - Turnstile's free tier allows 10 hostnames per widget, and reCAPTCHA now requires a Google Cloud project.
  - Private Access Tokens can be verified with the issuer's public key alone. But Apple issues them only to its own
    devices and only for a first-party challenge, so they cannot guard a cross-origin post to the Instance.
- **A form that sends a code by SMS is the textbook SMS-pumping target, and the sender pays.** Pumping is classed as
  OWASP OAT-003.
  - Twilio, AWS, Braze, Okta and the i3forum all put the cost on the account that sent the message. Twilio refunds
    only Verify traffic, and only in Fraud Guard's Max mode, after a claim it approves.
  - Klaviyo is the exception: it sends the code from its own short code and does not bill the brand.
  - A self-hosted Instance sending through the customer's own Twilio or AWS account puts the whole cost on the
    customer.
- **The vendors use four flows:**
  - a one-time code typed back into the form: Klaviyo Smart Opt-in, Postscript Onsite Opt-in;
  - a "Reply Y" prompt: Klaviyo, Postscript, Braze, Attentive;
  - mobile tap-to-text: the phone sends a prefilled keyword (Klaviyo, Attentive Two-Tap);
  - a QR code on desktop: Klaviyo for WhatsApp, Attentive in beta.

  Only tap-to-text and QR send nothing out before the person acts from their own phone.

- **Carriers.**
  - **CTIA's short-code handbook** says "Popups are not a method for displaying terms and conditions". It requires an
    opt-in confirmation message immediately after opt-in; double opt-in is optional.
  - **US 10DLC campaign review (Twilio)** wants a _public URL_ of the opt-in. A screenshot is accepted only when the
    form is behind a login or on paper. It also wants a separate, unchecked SMS checkbox, not bundled with email.
- **Before CMP consent, storage still needs a basis.** Under ePrivacy art. 5(3), writing a popup's state to a cookie or
  localStorage is "storage" whatever the data (Planet49; EDPB 2/2023).
  - The exemption for UI state covers what the user _explicitly asked_ the site to remember, for a session or a little
    longer (WP29 04/2012).
  - A cookie with several purposes is exempt only if every purpose is.
  - Klaviyo keeps forms working without consent by blocking only its tracking scripts.
  - sphericon's tracker today sets `om_vid` (365 days) and a localStorage queue on `init()`, with no consent hook.

### Per Channel: what a browser form can and cannot prove

| Channel  | What the form gives          | Confirming action the sources point to                                                  | Outbound cost before consent           |
| -------- | ---------------------------- | --------------------------------------------------------------------------------------- | -------------------------------------- |
| Email    | An address anyone could type | A click on a confirmation link (ADR 0013; DSK "geboten"; BGH)                           | One email, rate-limited (ADR 0013)     |
| SMS      | A number anyone could type   | A code sent to the number, a "Y" reply, or an MO keyword from the number (sections 3–4) | 0 (MO keyword) to 2+ (code, reminder)  |
| WhatsApp | A number anyone could type   | The person sends a prefilled message (wa.me or QR); Meta is silent on whether it counts | 0 for the MO; marketing templates bill |
| Telegram | Nothing to verify            | The person starts the bot; the Binding is the opt-in (ADR 0076)                         | 0                                      |
| Web push | Nothing to verify            | The browser permission after a click; the Binding is the opt-in (ADR 0075)              | 0                                      |

## 1. The setting in sphericon today

- **No consent hook in the tracker.** `packages/analytics/src/index.ts` posts to `${baseUrl}/collect` with the collect
  key as auth. On `init()` it:
  - sets the `om_vid` cookie (`expires: 365`, `sameSite: 'Lax'`) at once;
  - keeps unsent events in localStorage.
    It has no consent or opt-out API.
- **What the Confirmation records.** Its event carries when, how, provenance and IP (ADR 0013). On SMS and WhatsApp
  a Confirmation is always required (ADR 0077). It is recorded in one of three ways:
  - as `imported` from the customer's server, with the wording, source and brand;
  - as `keyword` when a START arrives on the Channel;
  - from an on-site form, "whose way of making a browser's consent trustworthy is decided with the forms" (ADR 0077).
- **Rate limiting already decided for email.** ADR 0013 already requires rate-limiting confirmation emails per
  Destination against subscription bombing.

## 2. Bot and abuse protection

### 2.1 Hosted challenges: an account and a call to the vendor [vendor]

| Product              | Account and secret                                                             | Verification                                                                           | Bound to hostnames                                                                     | Token                                   |
| -------------------- | ------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- | --------------------------------------- |
| Cloudflare Turnstile | Yes; "can be used independently without requiring other Cloudflare services"   | "You must call the Siteverify API"                                                     | Free: "a maximum of 10 hostnames per widget"; any-hostname widget is Enterprise only   | 300 s, "can only be validated once"     |
| hCaptcha             | Yes ("Your account secret key")                                                | `api.hcaptcha.com/siteverify`; pass `sitekey` so tokens cannot be "redeemed elsewhere" | "By default, a sitekey works on any domain"                                            | Single use, 120 s default               |
| reCAPTCHA v3         | Yes; since Q1 2026 "API access locked for keys without a Google Cloud project" | `google.com/recaptcha/api/siteverify`, returns a score 0.0–1.0                         | Tied to domains; validation can be switched off, then the server must check `hostname` | "valid for two minutes … verified once" |

Sources:

- Turnstile: [server-side validation](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/),
  [hostname management](https://developers.cloudflare.com/turnstile/concepts/hostname-management/),
  [plans](https://developers.cloudflare.com/turnstile/plans/).
- hCaptcha: [docs](https://docs.hcaptcha.com/), [configuration](https://docs.hcaptcha.com/configuration).
- reCAPTCHA: [v3](https://developers.google.com/recaptcha/docs/v3),
  [verify](https://developers.google.com/recaptcha/docs/verify),
  [domain validation](https://developers.google.com/recaptcha/docs/domain_validation),
  [migration](https://docs.cloud.google.com/recaptcha/docs/migration-overview).

Further facts:

- **Where verification happens.** All three verify server-to-server: the Instance's own domain does not matter. The
  hostname binding is between the widget and the _customer's_ site.
- **What a self-hosted Instance needs.** Each Workspace, or the operator, would need its own vendor account.
  - With Turnstile on the free tier, that means one widget per 10 customer hostnames.
  - reCAPTCHA's free tier is 10,000 assessments a month, with billing beyond that.
- **Privacy roles.**
  - Turnstile collects "client IP address, TLS Fingerprint, User-Agent Header and Sitekey and associated origin".
    Cloudflare is a processor for providing the service and a controller for improving bot detection
    ([privacy addendum](https://www.cloudflare.com/turnstile-privacy-policy/)).
  - hCaptcha says it "acts as a processor" for EU users ([FAQ](https://docs.hcaptcha.com/faq)).

### 2.2 Private Access Tokens (Privacy Pass) [spec, vendor]

- **Verifiable with no account.** Publicly verifiable tokens (type `0x0002`, blind RSA) "are publicly verifiable by
  anyone with the Issuer Public Key … without explicit coordination or permission from the Issuer"
  ([RFC 9578 §6](https://www.rfc-editor.org/rfc/rfc9578.html)).
  - Verification is a local RSASSA-PSS check.
  - Replay protection is the origin's job: "Origins SHOULD implement some form of double-spend prevention"
    ([RFC 9577 §2.2.2](https://www.rfc-editor.org/rfc/rfc9577.html)).
- **Apple devices only.** Tokens "require iOS 16 or macOS Ventura on a device that has an Apple ID signed in". Fastly
  limits them to "10 tokens per minute, per device"
  ([Fastly](https://docs.fastly.com/products/bot-management)).
- **No cross-origin challenge.** Apple: "the challenge must come from a first-party domain – a subdomain of your main
  URL and not a separate third-party domain embedded on your site"
  ([WWDC22 10077](https://developer.apple.com/videos/play/wwdc2022/10077/)).
  - A challenge from the Instance's domain to a script on the customer's site is therefore out.
  - Whether a CNAME of the customer's domain counts as first-party is not stated.
  - RFC 9577 says nothing about challenges on a `fetch()`.

### 2.3 Self-hosted proof-of-work [vendor]

- **ALTCHA.**
  - Described as "self-hosted, privacy-first", "cookie-free", "100% Self-Hosted … no reliance on third-party API
    availability", MIT licence ([repo](https://github.com/altcha-org/altcha)).
  - Go library [`altcha-lib-go/v2`](https://github.com/altcha-org/altcha-lib-go) (MIT):
    - `CreateChallenge` with an HMAC secret and a cost;
    - `VerifySolution` locally, with optional `ExpiresAt`.
  - Its README documents no replay store, so used challenges would have to be remembered until they expire
    (inference).
  - The hosted "Sentinel" is needed only for image and audio challenges.
- **Cap.**
  - "a self-hosted and privacy-first captcha alternative": a standalone server whose `/siteverify` "is compatible with
    reCAPTCHA's API"; "Tokens are single-use" ([guide](https://capjs.js.org/guide/),
    [repo](https://github.com/tiagozip/cap)).
  - The licence is Apache-2.0 per the README; GitHub reports `NOASSERTION`.
- **mCaptcha.** AGPL-3.0, "still very much work-in-progress", no releases, last push 2025-10
  ([repo](https://github.com/mCaptcha/mCaptcha)).
- **Friendly Captcha.** Needs its API (`global.frcapi.com`, `X-API-Key`). The self-hosted `friendly-lite-server` is PHP
  under a "non-commercial source available license"
  ([verify](https://developer.friendlycaptcha.com/docs/v2/getting-started/verify),
  [lite server](https://github.com/FriendlyCaptcha/friendly-lite-server)).

### 2.4 Measures without a challenge

- **OWASP Automated Threat Handbook v1.30**
  ([files](https://github.com/OWASP/www-project-automated-threats-to-web-applications/tree/master/assets/files/EN)):
  - "Form spam" is OAT-017. Its "Rate" countermeasure reads: "limit number and/or rate of usage per user, per group of
    users, per IP address/range, per device ID/fingerprint", with "upper and/or lower limits".
  - Form-to-SMS abuse "to inflate billable traffic" is OAT-003 Cost-Inflation Fraud ("SMS pumping; Toll fraud").
  - Its "Obfuscation" class includes "adding extra headers/fields dynamically, or adding page and session-specific
    tokens".
- **Honeypot fields and a minimum time to submit.** No primary source documents them as form defences. OWASP's
  "lower limits" is the closest. Treat them as common practice, **unverified**.
- **Proxies defeat per-IP limits.** Okta's threat advisory notes that SMS-pumping attackers use residential proxies,
  so per-IP limits are weak alone
  ([Okta](https://www.okta.com/blog/threat-intelligence/opportunistic-sms-pumping-attacks-target-customer-sign-up-pages/)).

## 3. SMS pumping (toll fraud) against a phone form

**What it is.** Twilio: "SMS pumping fraud happens when fraudsters take advantage of a phone number input field to
receive a one-time passcode". The fraudsters "get a share of the generated revenue"
([Twilio glossary](https://www.twilio.com/docs/glossary/what-is-sms-pumping-fraud)). The i3forum adds: "the
artificial generation of One-Time-Password SMS is the most straightforward and most used mechanism to generate AIT A2P
fraud" ([i3forum, Oct 2023](https://i3forum.org/wp-content/uploads/2023/12/i3forum_Fraud-Classification-Recommendations_fraud-dispute-handling_Oct2023.pdf)).
A popup that makes the Instance send a code, or a "Reply Y" prompt, to any typed number is that field.

### 3.1 Who pays [vendor, policy]

- **Twilio.** The customer is "solely responsible for all use of the Services … under your account"
  ([ToS §2.2(a)](https://www.twilio.com/en-us/legal/tos)), and pumped messages are "billed as usage to your account"
  ([anti-fraud guide](https://www.twilio.com/docs/usage/anti-fraud-developer-guide)).
  - **Verify credit.** The only credit is for Verify: Fraud Guard "must be enabled and set and maintained in Max
    Protection Mode". Twilio "will credit Customer any Verify fees and SMS fees incurred for those Fraudulent Messages",
    subject to "review and approval by Twilio" and a claim within 30 days after the month
    ([Fraud Guard terms](https://www.twilio.com/en-us/legal/service-country-specific-terms/verify-fraud-guard)).
  - **Plain Messaging.** No such guarantee was found for Programmable Messaging, the path a code sphericon generates
    itself would take.
  - **Unconverted Verify attempts.** The $0.05 fee is waived for failed verifications, but "some underlying delivery
    attempts may still incur channel charges" ([pricing](https://www.twilio.com/en-us/verify/pricing)).
- **AWS End User Messaging.** The Protect "Filter" mode costs "$0.01 per message in addition to standard SMS fees".
  "Customers will not be charged SMS per-message fees for messages blocked by SMS protect"
  ([announcement](https://aws.amazon.com/about-aws/whats-new/2025/04/aws-end-user-messaging-combat-sms-pumping/)).
  The default rule is "Allow" ([protect](https://docs.aws.amazon.com/sms-voice/latest/userguide/protect.html)).
- **Braze.** Customers "are invoiced for all SMS sent through their account"; the "Customer is the party in the better
  position to detect and prevent traffic pumping"
  ([best practices](https://braze.com/docs/user_guide/channels/sms_mms_and_rcs/compliance_and_delivery/best_practices)).
- **Okta.** "The victim organization is then billed for the exorbitant volume and cost" (advisory above).
- **i3forum §10.** If carriers cannot withhold payment downstream, "the liability remains with the originating service
  provider or Enterprise".
- **Klaviyo, the exception.** The Smart Opt-in code comes "from a short code managed by Klaviyo" and "will not count
  against your brand's Mobile Messaging spend, even if a shopper requests multiple authentication codes"
  ([Smart Opt-in](https://help.klaviyo.com/hc/en-us/articles/24743883751451)).
  - Klaviyo bears that cost (inference).
  - No resend limit or bot protection is documented.
  - The only country guidance is to show the form "only in countries where you have an active sending number".

### 3.2 Defences on offer

| Defence                                                  | Who offers it                                                                                                                                                                                                                                      | Default                                                                          |
| -------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| Limit per Destination                                    | Twilio Verify: "more than 5 times" ([60203](https://www.twilio.com/docs/api/errors/60203)); Auth0: "up to 10 SMS … per hour" per user; Bird: "a rolling cap of 5 sends per recipient address per hour"; Braze: one opt-in prompt per user per 24 h | On (window of Twilio's limit not stated)                                         |
| Limit per IP, session or custom key                      | Twilio [Programmable Rate Limits](https://www.twilio.com/docs/verify/api/programmable-rate-limits) ("IP address, session ID, or any other unique identifier"); Auth0: 50 per hour per IP on passwordless start                                     | Configured (Twilio); on (Auth0)                                                  |
| Allowed countries                                        | Twilio Messaging geo permissions: new accounts allow only "your home country"; Postscript Double Opt-In Allow List (23–26 countries); Clerk: "only the US and Canada"; Firebase SMS region policy; Braze geographic permissions                    | On at Twilio Messaging, Postscript, Clerk; AWS defaults to Allow                 |
| Prefix blocking by a risk model                          | Twilio Verify Fraud Guard ("blocks the prefix of the destination"; Basic, Standard as default level, Max); Twilio Messaging SMS Pumping Protection (free in US/CA; `RiskCheck` per message); AWS Filter; Vonage AIT; Plivo Fraud Shield            | Fraud Guard: docs say you are prompted at Service creation; others mostly opt-in |
| A challenge before the code                              | Firebase web phone auth requires a reCAPTCHA verifier; Auth0 Bot Detection; recommended by Twilio, Braze, Okta                                                                                                                                     | Required only at Firebase                                                        |
| Exponential delay between resends                        | Twilio (recommended); Auth0 "SMS Fraud Protection"                                                                                                                                                                                                 | Recommended                                                                      |
| Conversion-rate monitoring (codes verified ÷ codes sent) | Twilio (recommended); Vonage alerts at "35% or below" with an automatic network block; Firebase: "Success rates lower than 50% imply" abuse; Postscript: watch the ratio of prompts to opt-ins                                                     | Alerts, not blocks, except Vonage                                                |
| Send only to mobile lines                                | Twilio Lookup Line Type Intelligence (paid)                                                                                                                                                                                                        | Off                                                                              |
| Spend ceiling per account                                | Bird (planned); Okta (org-level cap)                                                                                                                                                                                                               | Rare                                                                             |
| The person sends first (MO keyword, QR)                  | Klaviyo tap-to-text and WhatsApp keyword; Attentive Two-Tap                                                                                                                                                                                        | Mobile only; removes the outbound cost before consent                            |

Sources:

- Twilio: [preventing toll fraud](https://www.twilio.com/docs/verify/preventing-toll-fraud),
  [Fraud Guard](https://www.twilio.com/docs/verify/preventing-toll-fraud/sms-fraud-guard),
  [SMS geo permissions](https://www.twilio.com/docs/messaging/guides/sms-geo-permissions),
  [SMS Pumping Protection](https://www.twilio.com/docs/messaging/features/sms-pumping-protection-programmable-messaging).
- Auth0: [passwordless best practices](https://auth0.com/docs/authenticate/passwordless/best-practices).
- Bird: [Verify security](https://bird.com/en-ca/products/verify/security).
- Postscript: [Double Opt-In Allow List](https://help.postscript.io/en/articles/13563860-double-opt-in-allow-list).
- Clerk: [sign-up options](https://clerk.com/docs/guides/configure/auth-strategies/sign-up-sign-in-options).
- Firebase: [multi-factor](https://firebase.google.com/docs/auth/web/multi-factor),
  [FAQ](https://firebase.google.com/docs/auth/faq-and-troubleshooting).
- Vonage: [AIT protection](https://developer.vonage.com/en/fraud-defender/guides/ait-protection).
- Plivo: [Fraud Shield](https://www.plivo.com/docs/verify/concepts/fraud-shield).
- Braze: [traffic pumping](https://www.braze.com/docs/user_guide/channels/sms_mms_and_rcs/compliance_and_delivery/sms_traffic_pumping_fraud).

**What no defence fixes.** Twilio: "No provider-side solution can guarantee 100% protection". Okta: the risk "can only
be fully mitigated by migrating to another authentication factor". A flow where the person's phone sends the first
message has no outbound leg to pump.

## 4. How vendors verify a phone typed into a popup [vendor]

| Vendor and flow                               | Messages sent before consent is confirmed                                     | Confirming action from the number        | Desktop                                                                 |
| --------------------------------------------- | ----------------------------------------------------------------------------- | ---------------------------------------- | ----------------------------------------------------------------------- |
| Klaviyo Smart Opt-in                          | 1 code (resends possible), from Klaviyo's short code, not billed to the brand | Code delivered to the number, typed back | "manually copy the code from their SMS messages into the form"          |
| Klaviyo tap-to-text (single opt-in keyword)   | 0: the phone sends the keyword                                                | MO from the number                       | Not shown; the phone field with Reply Y instead                         |
| Klaviyo, Postscript, Braze "Reply Y"          | 1 prompt (Braze: at most one per user per 24 h; 30 days to reply)             | MO "Y" from the number                   | Phone field                                                             |
| Attentive Two-Tap                             | The phone sends first, then 1 Reply-Y prompt by default                       | MO, then MO reply                        | Phone entry (inferred, article gated); QR "Two-tap for Desktop" in beta |
| Postscript Onsite Opt-in (default)            | 1 code + 1 fixed reminder 4 h later if unconfirmed                            | Code delivered to the number             | Code typed into the desktop form                                        |
| Klaviyo WhatsApp keyword (button, link or QR) | 0: "They must send the message to opt in"                                     | MO in WhatsApp                           | QR or link                                                              |

Klaviyo:

- **Smart Opt-in** ([24743883751451](https://help.klaviyo.com/hc/en-us/articles/24743883751451)) asks for the code "even
  if the destination list uses single opt-in" ([115005251108](https://help.klaviyo.com/hc/en-us/articles/115005251108)).
  Double opt-in is "strongly recommended … and in the US it is required if you plan to add SMS to your abandoned cart
  flows".
- **Tap-to-text** ([9351341171995](https://help.klaviyo.com/hc/en-us/articles/9351341171995)) may be single opt-in
  because "tap-to-text already asks the recipient to take 2 separate actions". Klaviyo advises not to publish
  single-opt-in keywords on the site ([360050384091](https://help.klaviyo.com/hc/en-us/articles/360050384091)).
- **WhatsApp** ([40116301104539](https://help.klaviyo.com/hc/en-us/articles/40116301104539)):
  - "opening the link or scanning the QR code alone does not subscribe them";
  - "Providing a phone number alone is not consent";
  - "use double opt-in to confirm the number is real and belongs to the person who signed up".
- **Consent record.** Klaviyo's API exposes `consent`, `consent_timestamp`, `method` and `method_detail`
  ([get_profile](https://developers.klaviyo.com/en/reference/get_profile)). It exposes no consent text, IP or form ID.

Postscript:

- **Onsite Opt-in** sends "a code sent via SMS to that phone number", plus a reminder "4 hours after", which cannot be
  customised ([13563921](https://help.postscript.io/en/articles/13563921-understanding-onsite-opt-in-confirmed-opt-in.md)).
- **Reply Y** prompt: "Shop Name: Reply Y to subscribe to recurring automated promotional msgs (e.g. cart reminders).
  Msg & data rates may apply." Its model popup copy has no checkbox: "By providing your number and clicking the
  button, you agree…" ([13564345](https://help.postscript.io/en/articles/13564345-what-is-the-tcpa-approved-compliance-language-i-need-to-use-when-collecting-subscribers.md)).

Attentive:

- **Default flow.** "Shopper taps phone number field → directed to Messages app → receives SMS → replies to confirm"
  ([53908818907668](https://help.attentive.com/hc/en-us/articles/53908818907668)).
- **API single opt-in.** Its API's `singleOptIn` skips the "Reply Y" text, but "The mandatory legal message will still
  be sent" ([addsubscriptions](https://docs.attentive.com/reference/addsubscriptions)).

Braze:

- **Double opt-in** "automatically works with the in-app message phone number capture form". A user has 30 days to
  confirm, and the confirmation reply is a configured message
  ([double opt-in](https://www.braze.com/docs/user_guide/channels/sms_mms_and_rcs/message_features_and_optimization/keyword_processing/double_opt_in),
  read through a summary).

Meta:

- Meta's docs say a user's first message opens a 24-hour customer service window
  ([pricing](https://developers.facebook.com/docs/whatsapp/pricing)). Service messages are charged per message from
  2026-10-01 beyond a free tier.
- Meta does **not** say whether a user-sent prefilled message (a `wa.me` link) is a _marketing_ opt-in. Klaviyo treats
  it as one; that is vendor practice.

## 5. What law and carriers require of a web-form opt-in

### 5.1 United States [law]

- **A website form is a writing.** "Consent obtained in compliance with the E-SIGN Act will satisfy the requirements of
  our revised rule, including permission obtained via an email, website form, text message, telephone keypress, or
  voice recording" (FCC 12-21 ¶34, [order](https://docs.fcc.gov/public/attachments/FCC-12-21A1.pdf)).
- **The seller proves it.** "Should any question about the consent arise, the seller will bear the burden of
  demonstrating that a clear and conspicuous disclosure was provided and that unambiguous consent was obtained"
  (¶33). Also: "the burden is on the caller to prove that it obtained the necessary prior express consent", and
  callers are expected to "maintain proper business records tracking consent" (FCC 15-72 ¶47, ¶70,
  [order](https://docs.fcc.gov/public/attachments/FCC-15-72A1.pdf)).
- **A number typed by someone else carries no consent.**
  - Of a number "entered incorrectly into a dialing system": "the caller never had valid prior express consent from
    the subscriber or customary user" (FCC 15-72 fn. 262).
  - An intermediary "cannot provide consent on behalf of the consumer", and if consent "was not, in fact, obtained,
    the sender … remains liable" (FCC 14-33 ¶11, ¶14, [GroupMe](https://docs.fcc.gov/public/attachments/FCC-14-33A1.pdf)).
  - FCC 15-72's reading of "called party" as the current subscriber was set aside for reassigned numbers by _ACA Int'l_
    (FCC 18-177 fn. 27).
- **No federal requirement to verify the number was found.** The reassigned-numbers safe harbor (64.1200(m)) covers
  reassignment, not a mistyped or hostile entry. FCC 26-67 changes revocation only, not how consent is obtained.

### 5.2 CTIA and US 10DLC [policy]

- **Opt-in mechanisms.** CTIA's Messaging Principles list "Entering a telephone number through a website" and
  "Clicking a button on a mobile webpage" as opt-in mechanisms. Opt-in "can also help prevent messages from being sent
  to a phone number that does not belong to the Consumer" (5.1.2; CCR 2.1 has the rest).
- **Short Code Monitoring Handbook v1.9** (in effect 2023-08-02,
  [PDF](https://api.ctia.org/wp-content/uploads/2024/01/CTIA-Short-Code-Monitoring-Handbook-v1.9-FINAL.pdf)):
  - Call-to-action contents: "Product description. / Message frequency disclosure. / Complete terms and conditions OR
    link to complete terms and conditions. / Privacy policy OR link to privacy policy. / STOP keyword. / 'Message and
    data rates may apply' disclosure."
  - Footnote 6: **"Popups are not a method for displaying terms and conditions."** Terms go beneath the call-to-action
    or behind a link next to it.
  - "The opt-in confirmation message must be delivered immediately after the Consumer opts into the Program", and
    "Recurring-messages Programs must send one message confirming opt-in consent".
  - "Double opt-in is optional."
- **Twilio's 10DLC campaign review** (error pages under [twilio.com/docs/api/errors](https://www.twilio.com/docs/api/errors)):
  - [30917](https://www.twilio.com/docs/api/errors/30917): "If opt-in happens on a website, provide the public URL … If
    opt-in happens on a paper form or behind a login, provide a public link to hosted screenshots, images, or a video."
  - [30543](https://www.twilio.com/docs/api/errors/30543): "A screenshot or mock-up does not replace the required live
    link" for a public website flow.
  - [30925](https://www.twilio.com/docs/api/errors/30925): "Add a separate checkbox or similar control for messaging
    consent. Leave it unchecked by default … Keep messaging consent separate from acceptance of general Terms of
    Service."
  - [30556](https://www.twilio.com/docs/api/errors/30556): consent "cannot be bundled" with email.
  - The campaign's `MessageFlow` must list every opt-in method
    ([UsAppToPerson](https://www.twilio.com/docs/messaging/api/usapptoperson-resource)).
  - Postscript's and Attentive's popups use "By providing your number and clicking the button…" without a checkbox.
    That conflict was not resolved.
  - No primary text of The Campaign Registry on web-form proof was found.

### 5.3 WhatsApp [policy]

- **Method.** "You are solely responsible for determining the method of opt-in"
  ([policy](https://whatsappbusiness.com/policy/), updated 2026-09-23). "Website" is one of the supported methods
  ([getting opt-in](https://developers.facebook.com/documentation/business-messaging/whatsapp/getting-opt-in)).
- **Silent points.** Nothing found on checkboxes, on separating WhatsApp consent from SMS consent, or on verifying the
  number.

### 5.4 Russia [law]

- **The FAS letter of 11.11.2019 N ДФ/98054/19 is marked lost force.** It is the source of "формы на сайтах … не
  позволяющих однозначно установить, кто именно дал согласие" (CCR 4.1). КонсультантПлюс now shows «Документ утратил
  силу или отменен» on it ([КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_337245/), checked
  2026-11-01). Two other letters are marked the same way:
  - its predecessor АК/65823/17;
  - АК/21587/13, which CCR cites for "в любой, в том числе в устной форме".
    No replacing letter was found.
- **The identifying-action rule stands on two texts in force:**
  - 126-FZ art. 44.1(1) for every рассылка: consent "выраженного посредством совершения им действий, однозначно
    идентифицирующих этого абонента и позволяющих достоверно установить его волеизъявление" (CCR 4.2);
  - Пленум ВАС N 58 of 08.10.2012, п. 15: consent "может быть выражено в любой форме, достаточной для его
    идентификации и подтверждения волеизъявления на получение рекламы от конкретного рекламораспространителя"; the
    абонент is "лицо, на чей адрес электронной почты или телефон поступило соответствующее рекламное сообщение"
    ([КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_137349/)).
- **Newer practice.**
  - FAS letter of 18.06.2026 N АТ/61279/26: consent's term "не ограничен … и при этом не должен быть бессрочным"
    ([КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_539057/)).
  - The joint Bank of Russia/FAS letter of 06.09.2021 (ИН-06-59/70, АК/75514/21) recommends a separate form on the
    site and a separate mark per named distributor ([cbr.ru](https://cbr.ru/StaticHtml/File/117596/20210906_in_06_59-70.pdf)).
  - The 2nd Arbitrazh Appeal Court (А29-16004/2025) rejected a consent with no way to refuse at signing
    ([court news](https://2aas.arbitr.ru/news/20260605-1)).
- **No source accepts an SMS code.** No FAS letter or court decision was found that accepts an SMS code or a reply
  from the number as the identifying action. 63-FZ art. 5(2) defines a simple electronic signature by "кодов,
  паролей", but under art. 6(2) it equals a handwritten one only where a law or an agreement between the parties says
  so ([КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_112701/)). No source applying it to
  advertising consent was found.
- **152-FZ.**
  - Since 2025-09-01 the personal-data consent "должно быть оформлено отдельно от иных информации и (или) документов"
    (art. 9(1), per 156-FZ art. 7(2)).
  - Data obtained from someone other than the subject needs "подтверждения наличия оснований" (art. 9(8)).
  - Source: [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_61801/).
- **Cookies.** No primary Roskomnadzor text on cookies or consent banners was found (**unverified**).

### 5.5 European Union and Germany [law]

- **No pre-ticked boxes.** "The use of pre-ticked opt-in boxes is invalid under the GDPR" (EDPB Guidelines 05/2020
  ¶79, [PDF](https://www.edpb.europa.eu/system/files/documents/files/file1/edpb_guidelines_202005_consent_en.pdf)).
  Planet49 ¶63 says the same for storage consent.
- **Proof.** "A controller could retain information on the session in which consent was expressed, together with
  documentation of the consent workflow at the time of the session, and a copy of the information that was presented
  to the data subject at that time. It would not be sufficient to merely refer to a correct configuration of the
  respective website" (¶108).
- **The controller demonstrates consent.** It must show "the data subject has, by active behaviour, given his or her
  consent" (C-61/19 _Orange România_, [judgment](https://publications.europa.eu/resource/celex/62019CJ0061)).
- **The verification-code paragraph is not about marketing.** EDPB ¶98 mentions "an SMS message with a verification
  code", but under _explicit_ consent (art. 9), not as a double opt-in rule for marketing.
- **BGH I ZR 164/09, 10.02.2011 (_Double-opt-in-Verfahren_)**, read on a mirror
  ([lexetius](https://lexetius.com/2011,4717); headnotes at [WIPO Lex](https://www.wipo.int/wipolex/en/text/597212)):
  - Each consent must be fully documented: stored, and printable at any time.
  - For email, a confirmed address plus a ticked box is "grundsätzlich hinreichend dokumentiert" (Rn. 45).
  - An email confirmation proves no consent to advertising calls (headnote c, translated).
  - The advertiser must prove that the phone line belongs to the confirmed email address (Rn. 48).
  - The court names "bewusste Falscheingabe in Belästigungs- und Schädigungsabsicht" as a cause of wrong entries
    (Rn. 47).
- **DSK Orientierungshilfe Direktwerbung (Feb 2022), 3.3**
  ([PDF](https://www.datenschutzkonferenz-online.de/media/oh/OH-Werbung_Februar%202022_final.pdf)):
  - For a consent declared electronically, the double opt-in procedure is "geboten" (required) to verify the
    person's declaration (translated).
  - Storing an IP address alone is not enough.
  - On SMS or phone codes: because they are easy to forge, the evidential value of such procedures is "in der Regel
    … gering" (as a rule low; translated).

## 6. Email: double opt-in from a form

- **What sphericon has: ADR 0013.**
  - A `double_opt_in` Confirmation is recorded by a `POST` from the confirmation page, so a scanner `GET` cannot
    fabricate consent.
  - Confirmation emails respect Suppression and an `everything` Unsubscribe, and are rate-limited per Destination.
  - It applies only when the Workspace's require-confirmed-opt-in policy is on. The default is off, so a form email is
    single opt-in and mailable at once.
- **Vendors** (prior research, sections 2–3):
  - Klaviyo: double opt-in per list.
  - Brevo: double opt-in as a form setting, with the GDPR text put in the confirmation email.
  - Braze: a separate Canvas with a confirmation link.
- **Mautic.** The 6.0 form actions include "Send Email to Contact" but no double opt-in
  ([forms](https://docs.mautic.org/en/6.0/components/forms.html)). It is offered by a third-party plugin and a roadmap
  proposal (**unverified**, not primary).
- **Law.**
  - Germany: double opt-in for email is "geboten" (DSK) and is sufficient documentation for email (BGH Rn. 45).
  - GDPR and ePrivacy require an active, provable consent, not double opt-in as such (5.5).

## 7. Cookie consent: may the popup show and keep state before a CMP accepts?

- **Showing a popup is not storage.** Rendering one stores nothing. Its frequency state, kept in a cookie or
  localStorage, is storage.
- **Art. 5(3).** It covers "the storing of information, or the gaining of access to information already stored, in the
  terminal equipment". The one exemption that fits is storage "strictly necessary in order for the provider of an
  information society service explicitly requested by the subscriber or user to provide the service" (2002/58/EC as
  amended by 2009/136/EC, CELEX 32009L0136).
- **Personal data or not.** It applies "regardless of whether or not it is personal data" (Planet49 ¶70–71,
  [CELEX 62017CJ0673](https://publications.europa.eu/resource/celex/62017CJ0673)).
- **EDPB Guidelines 2/2023 v2.0**
  ([PDF](https://www.edpb.europa.eu/system/files/documents/2024-10/edpb_guidelines_202302_technical_scope_art_53_eprivacydirective_v2_en_0.pdf)):
  - **Storage.** Writing through "browser cookie storage as well as customized software" is storage (¶35–36), and that
    covers localStorage.
  - **Local reads are not access.** Reading it inside the device is not "gaining of access" "as long as the
    information does not leave the device" (¶44).
  - **Sending it is access.** JavaScript that makes the browser "send asynchronous requests with the targeted
    information" "clearly falls within the scope" (¶33).
  - **Exemptions are out of scope.** The guidelines do not address the exemptions (¶4).
- **WP29 Opinion 04/2012**
  ([WP194](https://ec.europa.eu/justice/article-29/documentation/opinion-recommendation/files/2012/wp194_en.pdf)):
  - **UI customisation (§3.6).** Customisation cookies are exempt only if "set if the user has explicitly requested the
    service to remember a certain piece of information, for example, by clicking on a button or ticking a box", and
    only "session (or short term) cookies".
  - **User input (§3.1).** Session cookies for "filling online forms over several pages" are exempt.
  - **Several purposes (§2.4).** A cookie with several purposes "may only be exempted from consent if all the distinct
    purposes … are individually exempted".
  - **Frequency capping.** For third-party advertising, consent is needed even for "frequency capping".
  - **Popups.** The opinion never mentions popups.
  - Inference, not a source: a "don't show again" flag set by the visitor's click, short-lived and kept apart from
    `om_vid`, is the strongest case for the exemption. A frequency rule set by the marketer is not something the
    visitor requested.
- **How vendors handle it.**
  - **Klaviyo.** It serves tracking from `static-tracking.klaviyo.com` and forms from `static.klaviyo.com`, and advises
    blocking only the tracking scripts "so forms that do not depend on visitor tracking can still load"
    ([OneTrust guide](https://help.klaviyo.com/hc/en-us/articles/4764571493275)).
    - "Klaviyo forms, including pop-ups, can continue to appear if you block tracking"; `__kla_id` lives 2 years
      ([cookies](https://help.klaviyo.com/hc/en-us/articles/360034666712)).
    - How its forms store dismissal is not documented.
  - **CMP APIs a tracker can read:**
    - Google Consent Mode v2 (`gtag('consent', 'default' | 'update', …)`, with `functionality_storage` and
      `security_storage` among its types, [docs](https://developers.google.com/tag-platform/security/guides/consent));
    - Cookiebot (`Cookiebot.consent.{necessary,preferences,statistics,marketing}`, `CookiebotOnAccept`,
      [developer](https://www.cookiebot.com/en/developer/));
    - OneTrust (`OneTrust.OnConsentChanged`, `OnetrustActiveGroups`,
      [API](https://developer.onetrust.com/onetrust/docs/javascript-api));
    - IAB TCF v2 (`__tcfapi('addEventListener', 2, cb)`,
      [CMP API](https://github.com/InteractiveAdvertisingBureau/GDPR-Transparency-and-Consent-Framework/blob/master/TCFv2/IAB%20Tech%20Lab%20-%20CMP%20API%20v2.md)).

## Facts the grilling ticket (#954) depends on

Findings, not decisions.

1. **Recording a form's phone consent "as is".** `GLOSSARY.md` already files a consent from "a sphericon on-site
   form" under `imported`, beside the API and imports, with the wording, its source and the brand. That provenance
   describes consent a customer collected and asserts.
   - A browser submission carries no such guarantee: the collect key is public.
   - Under the sources above, such a record proves only that some browser submitted the number:
     - US: the seller's burden, and no consent if typed by another (5.1);
     - Russia: no identifying action (5.4);
     - Germany: double opt-in expected (5.5).
2. **An MO keyword fits ADR 0077 without new provenance.** A START sent from the number is already a `keyword`
   Confirmation in ADR 0077, and sphericon sends nothing before it.
   - A tap-to-text or QR flow whose prefilled message is a START keyword (plus a token tying it to the Visitor)
     therefore lands on an existing provenance.
   - It needs the Integration to receive MO messages, and a number or keyword the page can put in an `sms:` link or a
     `wa.me` link.
   - Desktop needs a QR code.
   - Whether a non-START join keyword also counts is open.
3. **Codes and Reply-Y prompts send before consent.** They:
   - need a non-marketing send before consent, which ADR 0077 rules out ("no non-marketing message is sent on these
     Channels");
   - expose the customer's provider account to pumping, with the customer paying (section 3);
   - in Germany, carry little evidential weight (DSK).
4. **Bot protection without a vendor account.**
   - Proof-of-work (ALTCHA-style HMAC challenges, verified locally in Go) plus per-Destination, per-IP and
     per-Workspace limits work on any Instance.
   - Hosted captchas need a per-Workspace or per-operator account and hostname registration.
   - Private Access Tokens cannot guard a cross-origin post.
   - Any challenge adds a step to the public collect path, which is an ADR 0036 contract.
5. **The disclosure travels with the message.**
   - The CTIA handbook wants terms beneath or linked next to the call-to-action, not in a popup.
   - 10DLC wants a separate unchecked SMS box and a live public URL of the opt-in. The customer's page is public, but
     an On-site message shows only when its display rules (audience, timing, frequency) let it, so a reviewer opening
     the URL may not see it.
   - 152-FZ wants the consent separate from other documents.
   - EDPB wants the exact text shown, kept per session.
   - Each asks for the exact consent text shown at the time; the `marketing.confirmed` Event does not carry it today
     (CCR 9).
6. **The tracker stores before any CMP answer.** `om_vid` and the localStorage queue are written on `init()` with no
   consent hook. A popup's frequency state would add a third store; the exemption is narrow (section 7).

## Open questions

1. What replaced the FAS letters ДФ/98054/19 and АК/65823/17, and does FAS still hold that a site form alone is
   inadequate? Is an SMS code or a reply accepted as the identifying action of 126-FZ art. 44.1? (Not found; FAS
   databases unreachable.)
2. Does a user-sent `wa.me` prefilled message count as a WhatsApp marketing opt-in under Meta's policy? (Meta is
   silent; Klaviyo treats it as one.)
3. Will a 10DLC reviewer accept the customer's public URL as the opt-in when the popup there shows only under its
   display rules (audience, exit intent, frequency), or does the review need a page where it always shows?
4. The CTIA handbook footnote ("Popups are not a method for displaying terms and conditions") against the vendors'
   popup copy, and Twilio's "separate checkbox" against Postscript's and Attentive's checkbox-less popups.
5. Whether a Private Access Token challenge from a CNAME of the customer's domain counts as first-party for Apple.
6. ALTCHA's replay guidance and exact protocol (altcha.org unreachable).
7. Twilio Fraud Guard's default state and the window of Verify's 5-send limit; Twilio Messaging SMS Pumping
   Protection's price outside the US and Canada.
8. Attentive's desktop flow and fraud controls (gated); whether Postscript's allow list also gates its code; whether
   Klaviyo limits code resends.
9. Roskomnadzor's position on cookies and consent banners.

**Caveat.** This is research from primary sources, not legal advice. A Workspace that collects SMS or WhatsApp
consent from a website needs counsel to review the wording, the evidence it keeps, and its carrier registration.
