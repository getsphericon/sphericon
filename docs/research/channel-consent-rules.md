# Consent rules for marketing SMS, messengers and push

Researched 2026-11-01 for #922 (map #916) from primary sources: the current eCFR text of 47 CFR 64.1200 (read
through the eCFR renderer API), Federal Register documents (FCC orders, read through the Federal Register API), the
CTIA Messaging Principles PDF, EU law from the EU Publications Office (Cellar, the text EUR-Lex serves), the CJEU
judgment, Russian law as consolidated by КонсультантПлюс (`consultant.ru`), FAS letters, Florida statutes, and the
first-party docs and policies of WhatsApp/Meta, Telegram, Apple, Google, Twilio, The Campaign Registry and the W3C/IETF.
No blogs or law-firm summaries are relied on. A claim not confirmed in a primary source is marked **unverified**.

**Law or policy.** Every rule below is tagged **[law]** (statute, regulation, court) or **[policy]** (a platform, carrier
or industry rule, enforced by blocking or filtering rather than by a regulator). **[spec]** marks a technical standard.

**Limits.** ICO pages and Twilio's help-centre articles returned 403 or render only with JavaScript, so no ICO claim is
made and Twilio's opt-out scope is marked unverified. EUR-Lex blocks scripted fetches; the ePrivacy text quoted is the
2002 original from Cellar (art. 13 was later amended by Directive 2009/136/EC, which the CJEU cites). Member-state
implementations of ePrivacy art. 13 and US state laws other than Florida were not researched. **This is research, not
legal advice.** A Workspace sending to these channels needs its own counsel.

Terms as in `GLOSSARY.md` (section "Send-eligibility"): **Destination**, **Suppression**, **Unsubscribe**, **Sending
source**, **Confirmation**; ADR 0013 for the `marketing.confirmed` Event.

## Question

What consent and opt-out rules must a sender honor per channel, so Send-eligibility can encode them? For each channel:
is prior express consent required for marketing; is soft opt-in allowed; which opt-out mechanisms are required and by
when; quiet hours; what proves consent; and does an opt-out cover every message from the business?

## Summary table

| Channel / region      | Prior consent for marketing                                                  | Soft opt-in                                      | Opt-out: how and by when                                                                                 | Opt-out scope                                                       | Quiet hours                                                                 |
| --------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------ | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| SMS, US               | Written consent if autodialed [law]; express written consent always [policy] | No (EBR only exempts from DNC, not from consent) | STOP and 6 other words; ≤10 business days [law]; one confirmation text [law]; "act on" [policy]          | All marketing from that caller (from 2026-11-25) [law]              | 8:00–21:00 local, unconsented solicitations only [law]; FL 8:00–20:00 calls |
| SMS, EU               | Prior consent [law]                                                          | Yes: own customer, own similar products [law]    | Free, easy objection in every message [law]; GDPR: as easy as giving consent                             | Direct marketing as a whole (GDPR art. 21) [law]                    | None at EU level                                                            |
| SMS, Russia           | Prior consent, provable, uniquely identifying the subscriber [law]           | No                                               | "Immediately" on request [law]; operator-level blanket refusal, effective next calendar day [law]        | All ads from that distributor [law]; operator refusal: all рассылки | None found                                                                  |
| WhatsApp              | Opt-in naming the business [policy] plus the law of the recipient's country  | No                                               | Honor "all requests, on or off WhatsApp" [policy]; user "stop marketing" in-app, error `131050` [policy] | User stop covers all marketing templates of the business [policy]   | None                                                                        |
| Telegram              | The user must message the bot first [policy/technical]                       | n/a                                              | User blocks the bot: `my_chat_member` update [technical]                                                 | Whole bot                                                           | None                                                                        |
| Mobile push (iOS)     | OS permission, plus explicit in-app consent for marketing [policy]           | n/a                                              | In-app opt-out required for marketing [policy]; OS settings                                              | Marketing (in-app), all (OS)                                        | None                                                                        |
| Mobile push (Android) | OS runtime permission on Android 13+ [technical]                             | n/a                                              | OS settings; Play policy on spam [policy]                                                                | All (OS) or per notification channel                                | None                                                                        |
| Web push              | Browser permission after a user gesture (Safari/iOS) [spec/technical]        | n/a                                              | Browser settings; subscription ends, push service answers `404` [spec]                                   | Whole subscription                                                  | None                                                                        |

EU law for messengers and push is unsettled; see 3.4 and 3.5. Russia's 38-FZ applies to all "сети электросвязи"
(electronic communications networks), not just SMS; see 4.1.

## 1. United States: TCPA and the FCC rules [law]

### 1.1 Consent

- **Autodialed or prerecorded marketing needs prior express _written_ consent.** No one may "initiate … any telephone
  call that includes or introduces an advertisement or constitutes telemarketing, using an automatic telephone dialing
  system or an artificial or prerecorded voice" to a cellular number "other than a call made with the prior express
  written consent of the called party" (47 CFR 64.1200(a)(2),
  [eCFR](https://www.ecfr.gov/current/title-47/chapter-I/subchapter-B/part-64/subpart-L/section-64.1200)). The FCC's
  rules treat text messages as calls here: the revocation rule speaks of "calls or text messages made pursuant to
  paragraphs (a)(1) through (3)" (64.1200(a)(10), same source).
- **What "prior express written consent" is** (64.1200(f)(9)): "an agreement, in writing, bearing the signature of the
  person called that clearly authorizes the seller to deliver … advertisements or telemarketing messages using an
  automatic telephone dialing system or an artificial or prerecorded voice, and the telephone number to which the
  signatory authorizes such advertisements". It must disclose clearly and conspicuously that the person authorizes such
  messages and "is not required to sign the agreement … as a condition of purchasing any property, goods, or services".
  An electronic signature valid under federal or state law counts.
- **The written-consent rule depends on the equipment.** An "automatic telephone dialing system" is "equipment which has
  the capacity to store or produce telephone numbers to be called using a random or sequential number generator and to
  dial such numbers" (64.1200(f)(2)). A platform that texts a stored list may fall outside that definition, so federal
  law alone does not require written consent for every marketing SMS. The industry rules in section 2 do. Whether a
  given sending system is an autodialer is a legal question this note does not answer.
- **"One-to-one" consent is gone.** The 2023 rule that consent apply "to a single seller at a time"
  ([89 FR 87982](https://www.federalregister.gov/documents/2024/11/06/2024-24908/rules-and-regulations-implementing-the-telephone-consumer-protection-act-of-1991))
  was vacated by a court; the FCC removed it as of the court's mandate of 2025-04-30
  ([90 FR 42137](https://www.federalregister.gov/documents/2025/08/29/2025-16641/delete-delete-delete-targeting-and-eliminating-unlawful-text-messages-rules-and-regulations)).
- **Do-not-call rules apply to texts regardless of equipment.** The time-of-day and do-not-call rules in (c) and (d)
  "are applicable to any person or entity making telephone solicitations or telemarketing calls or text messages to
  wireless telephone numbers" (64.1200(e)); the FCC "codifies that the National Do-Not-Call (DNC) Registry's protections
  extend to text messages"
  ([89 FR 5098](https://www.federalregister.gov/documents/2024/01/26/2023-28832/targeting-and-eliminating-unlawful-text-messages-implementation-of-the-telephone-consumer-protection)).
  A "telephone solicitation" excludes a message "to any person with that person's prior express invitation or
  permission" or "with whom the caller has an established business relationship" (64.1200(f)(15)). The established
  business relationship (purchase within 18 months, inquiry within 3 months) ends with the subscriber's seller-specific
  do-not-call request "even if the subscriber continues to do business with the seller" (64.1200(f)(5)(i)).

### 1.2 Revocation: the rule today and the rule from 2026-11-25

**In force on 2026-11-01** (64.1200(a)(10)–(12), eCFR):

- Consent may be revoked "by using any reasonable method". Replying "stop," "quit," "end," "revoke," "opt out,"
  "cancel," or "unsubscribe" to a text, an automated opt-out on a call, or a website or number the caller designates is
  "a reasonable means per se". Other words count "if a reasonable person would understand those words to have conveyed
  a request to revoke consent".
- A sender using a protocol that cannot receive replies must say so in each text and give "reasonable alternative ways
  to revoke consent" in each text.
- Revocations must be honored "within a reasonable time not to exceed ten business days from receipt"; callers "may not
  designate an exclusive means to request revocation".
- Any other means, "such as a voicemail or email to any telephone number or email address intended to reach the
  caller", creates a rebuttable presumption that consent was revoked (a)(11).
- A one-time confirmation text is allowed if it "merely confirms the text recipient's revocation request and does not
  include any marketing or promotional information, and is the only additional message"; within five minutes it is
  presumed covered by consent; it "may request clarification" of scope when the person consented to several categories
  (a)(12).

**From 2026-11-25** (FCC 26-67, Report and Order,
[91 FR 67214](https://www.federalregister.gov/documents/2026/10/26/2026-21821/enhancing-the-ability-of-consumers-to-control-the-robocalls),
"DATES: Effective November 25, 2026"):

- "Callers may designate any of the following methods as the exclusive means to revoke consent": a voice/key-press
  opt-out, "replying to an incoming text message with any one of the following standardized words: 'stop,' 'quit,'
  'end,' 'revoke,' 'opt out,' 'cancel,' or 'unsubscribe'", or a designated website or number. The method must be
  disclosed "on the call or in the text"; a texter may name just one word (e.g. "stop") "so long as the text sender
  recognizes the use of any of the standardized words". A caller that designates none "must process revocation
  requests made by any reasonable means" (new (a)(11)).
- **Scope.** "Revocation requests made in response to calls or text messages that contain an advertisement or
  constitute telemarketing revoke consent to all future calls or text messages containing an advertisement or
  constitute telemarketing from that caller." A revocation sent in reply to an _informational_ message may be read as
  covering only "the specific category of informational robocalls and text messages to which the revocation was
  directed". This supersedes the 2024 "revoke all" sentence whose effective date had been pushed to 2027-01-31 (same
  document, section E).
- The deadline stays at ten business days; the confirmation text keeps the five-minute presumption (the new (a)(12)
  drops "the only additional message").
- The order also reminds that a do-not-call request applies to the calling entity "as well as affiliated entities if
  the consumer reasonably would expect them to be included" (91 FR 67214, section A). The rule itself says the request
  "will not apply to affiliated entities unless the consumer reasonably would expect them to be included"
  (64.1200(d)(5)).

**Pending.** A Further Notice
([91 FR 67239](https://www.federalregister.gov/documents/2026/10/26/2026-21820/modernizing-the-ability-of-consumers-to-control-robocalls),
comments due 2026-11-25, replies 2026-12-28) asks about cutting the deadline to seven business days, requiring two-way
texting, requiring a "revoke all" method, and the treatment of affiliates.

### 1.3 Quiet hours

- No telephone solicitation "before the hour of 8 a.m. or after 9 p.m. (local time at the called party's location)"
  (64.1200(c)(1)), applied to texts by (e). Because a "telephone solicitation" excludes messages sent with prior express
  permission or an established business relationship ((f)(15)), the federal window binds only unconsented marketing
  texts. **[law]**
- Florida: "A commercial telephone solicitation phone call before 8 a.m. or after 8 p.m. local time in the called
  person's time zone" and "more than three commercial telephone solicitation phone calls from any number to a person
  over a 24-hour period on the same subject matter" are forbidden (Fla. Stat. 501.616(6),
  [text](http://www.leg.state.fl.us/statutes/index.cfm?App_mode=Display_Statute&URL=0500-0599/0501/Sections/0501.616.html)).
  The section speaks of phone calls; whether it reaches texts is **unverified**. Florida's Telephone Solicitation Act
  does reach texts: before suing, the recipient must reply "STOP", and "within 15 days after receipt of such notice,
  the telephone solicitor shall cease sending text message solicitations" (Fla. Stat. 501.059(10)(c),
  [text](http://www.leg.state.fl.us/statutes/index.cfm?App_mode=Display_Statute&URL=0500-0599/0501/Sections/0501.059.html)).
  Other states were not researched.

### 1.4 Records

- A do-not-call request "must be honored for 5 years from the time the request is made", and the sender "must maintain
  a record of a consumer's request not to receive further calls" (64.1200(d)(6)). The request is recorded "at the time
  the request is made" and must be honored within ten business days (d)(3). A DNC request applies to "the particular
  entity making the call (or on whose behalf a call is made)" and to affiliates the consumer would reasonably expect
  (d)(5).

## 2. United States: carrier and industry rules for SMS [policy]

### 2.1 CTIA Messaging Principles and Best Practices (May 2023)

Source: [CTIA PDF, May 2023](https://api.ctia.org/wp-content/uploads/2023/05/230523-CTIA-Messaging-Principles-and-Best-Practices-FINAL.pdf),
the latest edition found (section 5.1, pp. 10–13). Not law: carriers enforce it through vetting and filtering.

- **Consent levels.** Non-consumer senders are expected to "obtain a Consumer's express written consent to specifically
  receive marketing messages". Exhibit II: conversational (consumer texts first) = implied consent; informational =
  express consent; promotional = express written consent ("sign a form, check a box online, or otherwise provide
  consent"). "Adding a call-to-action (e.g., a coupon code to an informational text) may place the message in the
  promotional category."
- **Call-to-action.** The opt-in must show the program description, the sending number(s), the organization's identity,
  "clear and conspicuous language about opt-in and any associated fees or charges", and terms such as how to opt out,
  customer care contact and privacy policy; opt-in details "should not be obscured in terms and conditions" (5.1.1).
- **Proof of consent.** Retain "where applicable": timestamp; acquisition medium; "capture of experience (e.g., language
  and action) used to secure consent"; "specific campaign for which the opt-in was provided"; IP address; the phone
  number; identity of the consenting individual (name, user name, session ID) (5.1.2).
- **Opt-in confirmation.** For recurring campaigns, send a confirmation "before any additional messaging is sent", with
  the program name, customer care contact (or HELP instructions), how to opt out, that messages recur and how often, and
  any charges (5.1.2.1).
- **One opt-in per campaign.** An opt-in "should not be transferable or assignable" and applies "only to the campaign(s)
  and specific Message Sender for which it was intended or obtained" (5.1.2.2). Rented, sold or shared lists must not be
  used (5.1.4).
- **Opt-out.** Senders should support opt-out by phone, email or text, send "one final opt-out confirmation message per
  campaign", and send nothing after it. "Standardized 'STOP' wording should be used", but "stop, end, unsubscribe,
  cancel, quit, 'please opt me out'" "should also be read and acted upon", regardless of case or punctuation (5.1.3).
- **Records.** "Retain and maintain all opt-in and opt-out requests", and process deactivation files "regularly (e.g.,
  daily)" to drop deactivated numbers (5.1.5).

### 2.2 A2P 10DLC and The Campaign Registry

- Sending application traffic over US 10-digit long codes requires registering a **Brand** and a **Campaign**; the
  Campaign must describe how users opt in, opt out and get help, and carries sample messages
  ([Twilio, A2P 10DLC](https://www.twilio.com/docs/messaging/compliance/a2p-10dlc)). Unregistered traffic pays carrier
  fees and is filtered more (same source).
- The Campaign Registry calls itself "the backbone support for the 10DLC messaging ecosystem"; brands register only
  through a Campaign Service Provider ([TCR](https://www.campaignregistry.com/)). The registered Campaign is where the
  opt-in flow is declared, so the opt-in a Workspace collects must match what its provider registered.

### 2.3 Provider behaviour (Twilio as the example)

- Default opt-out keywords: "STOP", "UNSUBSCRIBE", "END", "QUIT", "STOPALL", "REVOKE", "OPTOUT", "CANCEL"; "HELP" and
  "START"/"UNSTOP" are reserved; a send to an opted-out recipient fails with error `21610`
  ([Twilio Advanced Opt-Out](https://www.twilio.com/docs/messaging/tutorials/advanced-opt-out)).
- The block is kept per sender number, or per Messaging Service when the send names one; toll-free STOP is handled by
  the carrier (Twilio help article
  [223134027](https://help.twilio.com/articles/223134027-Twilio-support-for-opt-out-keywords-SMS-STOP-filtering-),
  JS-rendered, **unverified**). So the provider's block is narrower than the legal scope in 1.2: sphericon must keep its
  own record.

## 3. European Union: ePrivacy Directive and GDPR [law]

### 3.1 SMS is "electronic mail"; marketing needs prior consent

- "Electronic mail" is "any text, voice, sound or image message sent over a public communications network which can be
  stored in the network or in the recipient's terminal equipment until it is collected by the recipient" (Directive
  2002/58/EC art. 2(h), [text](https://publications.europa.eu/resource/cellar/cb5af945-9d9f-40e6-acee-bd0c5bb4ed0a.0004.01/DOC_1)).
  Recital 40 names "e-mails, including SMS messages".
- "The use of … electronic mail for the purposes of direct marketing may only be allowed in respect of subscribers who
  have given their prior consent" (art. 13(1), same source).
- The CJEU reads "electronic mail" broadly: ads shown in a webmail inbox "in a form similar to that of real emails"
  constitute "use of … electronic mail for the purposes of direct marketing", permitted only if the user "indicated
  specific and fully informed consent" (C-102/20 _StWL Städtische Werke Lauf_, 25 Nov 2021,
  [judgment](https://publications.europa.eu/resource/cellar/28a7836c-4dec-11ec-91ac-01aa75ed71a1.0006.02/DOC_1)).

### 3.2 Soft opt-in

Art. 13(2): where a person "obtains from its customers their electronic contact details for electronic mail, in the
context of the sale of a product or a service", "the same natural or legal person may use these electronic contact
details for direct marketing of its own similar products or services provided that customers clearly and distinctly
are given the opportunity to object, free of charge and in an easy manner … when they are collected and on the occasion
of each message". Because SMS is electronic mail, the exception applies to SMS too. Three conditions the model would
have to carry: a sale (not just a sign-up), the same seller, similar products. Member states transpose this
differently; not researched.

### 3.3 Opt-out, identity and GDPR

- Marketing electronic mail must not disguise "the identity of the sender on whose behalf the communication is made" and
  must carry "a valid address to which the recipient may send a request that such communications cease" (art. 13(4)).
- GDPR: the controller "shall be able to demonstrate that the data subject has consented"; consent may be withdrawn at
  any time and "it shall be as easy to withdraw as to give consent" (Regulation (EU) 2016/679 art. 7(1), 7(3),
  [text](https://publications.europa.eu/resource/cellar/3e485e15-11bd-11e6-ba9a-01aa75ed71a1.0006.01/DOC_1)).
- The objection to direct marketing is absolute: "Where the data subject objects to processing for direct marketing
  purposes, the personal data shall no longer be processed for such purposes" (art. 21(3)); the right must be brought
  to attention "at the latest at the time of the first communication" (art. 21(4)). The objection covers direct
  marketing as a purpose, not one campaign.
- The only EU-level deadline found is GDPR's general one for acting on a data-subject request: "without undue delay
  and in any event within one month of receipt of the request" (art. 12(3)); ePrivacy sets none for SMS opt-outs. No
  EU-level quiet hours exist (national rules not researched).

### 3.4 Messengers (WhatsApp, Telegram): ePrivacy probably applies [inference]

Directive (EU) 2018/1972 (the European Electronic Communications Code) defines "electronic communications service" to
include "interpersonal communications service", and that includes the "number-independent" kind, one that "does not
connect with publicly assigned numbering resources" (art. 2(4), 2(5), 2(7),
[text](https://publications.europa.eu/resource/cellar/4bb873d0-01d2-11e9-adde-01aa75ed71a1.0006.01/DOC_1)). It applies
from 21 December 2020, and "references to the repealed Directives shall be construed as references to this Directive"
(art. 124, 125). The ePrivacy Directive's art. 2 takes its definitions from "Directive 2002/21/EC of the European
Parliament and of the Council of 7 March 2002 on a common regulatory framework for electronic communications networks
and services" (2002/58/EC art. 2), one of the repealed Directives, so it now reaches messenger apps; with C-102/20's broad reading of "electronic mail", a marketing message in WhatsApp or Telegram
is most likely art. 13 electronic mail needing prior consent. That chain is this note's inference, not a holding.

### 3.5 Push notifications: unresolved

No primary EU source was found saying whether an app or web push notification is "electronic mail" under art. 13.
GDPR still applies to the processing behind a targeted push (art. 6, art. 21). Treat the push channel's own consent
(OS or browser permission, plus Apple's in-app consent for marketing, section 7) as the floor.

## 4. Russia [law]

### 4.1 38-FZ "On Advertising", art. 18: prior consent, proof, immediate stop

- "Распространение рекламы по сетям электросвязи, в том числе посредством использования телефонной, факсимильной,
  подвижной радиотелефонной связи, допускается только при условии предварительного согласия абонента или адресата на
  получение рекламы." The ad is deemed sent without consent "если рекламораспространитель не докажет, что такое
  согласие было получено", and the distributor "обязан немедленно прекратить распространение рекламы в адрес лица,
  обратившегося к нему с таким требованием" (art. 18(1),
  [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_58968/f892dec1383709792452f18d36e7043306e2be0a/),
  edition of 04.08.2026). No soft opt-in exists; the stop is "immediately".
- Art. 18(2): "Не допускается использование сетей электросвязи для распространения рекламы с применением средств выбора
  и (или) набора абонентского номера без участия человека (автоматического дозванивания, автоматической рассылки)." Read
  literally, this bans automated ad sending even with consent. How FAS and the courts apply part 2 to a consented,
  automated campaign was **not resolved** in this research; it is material and needs counsel.
- **What proves consent (FAS).** Consent must let one "однозначно идентифицировать такого абонента" and confirm "его
  волеизъявление на получение рекламы от конкретного рекламораспространителя"; consent "полученное посредством заполнения
  каких-либо форм на сайтах … не позволяющих однозначно установить, кто именно дал согласие" is inadequate; a signed
  written contract containing consent may be adequate (FAS letter of 11.11.2019 N ДФ/98054/19,
  [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_337245/), citing Plenum of the Supreme Arbitrazh
  Court resolution N 58 of 08.10.2012, para. 15). The same letter lists push notifications alongside SMS and calls as
  media for art. 18(1) complaints. Consent "может быть … получено в любой, в том числе в устной форме" (FAS letter of
  04.06.2013 N АК/21587/13, [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_159541/)), but the burden
  of proof makes a recorded, identifying action the practical requirement.

### 4.2 126-FZ "On Communications", art. 44.1: mass SMS (рассылка)

- "Рассылка" is "автоматическая передача абонентам коротких текстовых сообщений" over the mobile network, or from
  numbering outside the Russian plan (art. 2(22.1),
  [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_43224/b819c620a8c698de35861ad4c9d9696ee0c3ee7a/)).
  FAS: this covers messages from "коротких" or "буквенных" (alphanumeric) numbers and automatically sent messages (FAS
  letter of 05.12.2014 N АК/49919/14, [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_177167/)).
- Prior consent "выраженного посредством совершения им действий, однозначно идентифицирующих этого абонента и
  позволяющих достоверно установить его волеизъявление на получение рассылки"; the customer of the рассылка bears the
  proof (art. 44.1(1),
  [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_43224/7caf446de1e18caa61180692ea35572c94080475/)).
  Unlike 38-FZ, this covers every рассылка, not only ads.
- The рассылка is sent "на основании договора, заключенного с оператором подвижной радиотелефонной связи, абоненту
  которого предназначена рассылка" (art. 44.1(2)). Operators must "прекратить оказание услуг по пропуску по своей сети
  трафика, содержащего осуществляемую с нарушением требований настоящего Федерального закона рассылку" (art. 46(1),
  [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_43224/16bb16c212b64fca3cd16dac9f4f1f2c2fd34083/)).
- **Operator-level refusal, since 2025-08-01.** "Абонент … вправе направить оператору подвижной радиотелефонной связи
  отказ от получения рассылки. Оператор … обязан прекратить рассылку" (art. 44.1(1.1), added by 41-FZ of 01.04.2025, in
  force 01.08.2025 per its art. 16(2),
  [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_502182/9aa36c9f3bc7f73f1c2d3a2327385ee820acfc07/)).
  The procedure: a statement in person or online, including the operator's self-service; the operator stops "рассылку,
  осуществляемую в соответствии со статьей 44.1" (all of them, except the exempt state and porting messages of
  art. 44.1(3)); the stop date "является календарный день, следующий за днем подачи заявления"; the subscriber may
  cancel the refusal, restored within the next calendar day (Rules of telephone services, Government Decree of
  30.12.2024 N 1994, paras. 217–221, added by Decree of 31.07.2025 N 1137,
  [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_533771/051dcbbcd870aa9428dd0978f759edd12e5bd2cd/)).
  This refusal is blanket across all senders and is held by the operator, not by the sender: a sender learns of it only
  as a failed delivery.
- **Sender-name registration.** No primary source was found, as of 2026-11-01, for a statutory registry of
  alphanumeric sender names in Russia; the legal hook is the per-operator contract of art. 44.1(2). Operator-side name
  registration is contractual practice (**unverified**, not researched). Federal Law of 26.06.2026 N 210-FZ amends
  126-FZ from 2027-03-01 (mass calls, contract terms); its text read here does not touch art. 44.1
  ([КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_537891/3d0cac60971a511280cbba229d9b6329c07731f7/)).

### 4.3 152-FZ "On Personal Data"

- Processing "в целях продвижения товаров, работ, услуг на рынке путем осуществления прямых контактов с потенциальным
  потребителем с помощью средств связи" needs prior consent, proven by the operator; the operator "обязан немедленно
  прекратить" such processing on request (art. 15(1)–(2),
  [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_61801/5656527e0713bf229a6932ac7084dec50d0ebe1f/)).
- Consent must be "конкретным, предметным, информированным, сознательным и однозначным", may be given "в любой
  позволяющей подтвердить факт его получения форме", and "должно быть оформлено отдельно от иных информации и (или)
  документов" (art. 9(1) as edited by 266-FZ of 14.07.2022 and 156-FZ of 24.06.2025; which of the two added the
  separate-document sentence was not confirmed); the burden of proof is on the operator (art. 9(1), 9(3),
  [КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_61801/6c94959bc017ac80140621762d2ac59f6006b08c/)).
  For the model: the marketing consent text must be its own document or checkbox, not a clause of terms.

### 4.4 Foreign messengers (41-FZ art. 15)

Using foreign-owned messengers "для информирования граждан Российской Федерации" is forbidden to listed organizations:
state bodies and state-owned companies, credit and non-credit financial organizations, telecom operators, owners of
goods/services aggregators, large social platforms and classified-ad services (41-FZ art. 15(1),
[КонсультантПлюс](https://www.consultant.ru/document/cons_doc_LAW_502182/244fc8fa7b8d53dba1dd85e469f239c26c7152b6/);
in force 01.06.2025 per art. 16(1)). This is a constraint on which Workspaces may use WhatsApp or Telegram to reach
Russian citizens, not a per-message eligibility rule.

### 4.5 Quiet hours

None found in 38-FZ art. 18, 126-FZ art. 44.1 or 152-FZ for marketing messages.

## 5. WhatsApp [policy]

- "You may only contact people on WhatsApp if: (a) they have given you their mobile phone number or username; and (b)
  you have received opt-in permission from the recipient confirming that they wish to receive subsequent messages or
  calls from you." Best practice: "Obtaining separate opt-in by specific message category"
  ([WhatsApp Business Messaging Policy](https://whatsappbusiness.com/policy/), last updated 2026-09-23).
- "You must respect all requests (either on or off WhatsApp) by a person to block, discontinue, or otherwise opt out"
  (same source). Users "can block or report businesses"; negative feedback can limit or remove access (section 7).
- The opt-in must "clearly state that a person is opting in to receive communication from the business" and "the
  business's name"; it may be collected by SMS, website, IVR, in person or on paper; businesses "must comply with
  applicable law" ([Meta, Getting opt-in](https://developers.facebook.com/docs/whatsapp/overview/getting-opt-in)).
- Business-initiated messages must be approved templates; free-form replies only within the 24-hour customer service
  window (policy section 2). Outside it the API fails with `131047`
  ([error codes](https://developers.facebook.com/docs/whatsapp/cloud-api/support/error-codes)).
- **User-side marketing stop.** A user can stop marketing messages from a business; the `user_preferences` webhook
  reports `value: stop` or `resume`, `category: marketing_messages`, keyed by `wa_id` or a business-scoped user ID
  ([webhook reference](https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/reference/user_preferences)).
  A marketing send to such a user fails with `131050` "This recipient has chosen to stop receiving marketing messages on
  WhatsApp from your business"; `131049` is a per-user marketing frequency limit; `130403` means the business blocked
  the user (error codes page).
- WhatsApp usernames and business-scoped user IDs mean the Destination may not be a phone number (`wa_id` "omitted if
  the user has enabled usernames", webhook reference).

## 6. Telegram [policy, technical]

- "Bots can't start conversations with users. A user must either add them to a group or send them a message first"
  ([Telegram Bots](https://core.telegram.org/bots)). A `t.me/<bot>?start=<param>` deep link delivers `/start <param>`
  to the bot ([Bot features](https://core.telegram.org/bots/features)), so the opt-in and the Destination (chat id) are
  born together and the start parameter can carry the Contact's identity.
- Bot Developer Terms: a bot "must not harass or spam users with unsolicited messages" (5.2(b)); bots may broadcast "up
  to 30 messages per second to users who have granted permission to contact them" (6.2.5)
  ([Bot Developer Terms](https://telegram.org/tos/bot-developers)). Paid broadcasts raise this to 1000/s
  ([Bots FAQ](https://core.telegram.org/bots/faq)).
- Opt-out is the block: "For private chats, this update [`my_chat_member`] is received only when the bot is blocked or
  unblocked by the user" ([Bot API](https://core.telegram.org/bots/api), Bot API 10.3, 2026-08-24). Telegram itself
  requires no in-message opt-out text; EU and Russian law (sections 3.4, 4.1) may.

## 7. Mobile push: Apple and Google [policy, technical]

- **Apple, App Review Guideline 4.5.4:** "Push Notifications should not be used for promotions or direct marketing
  purposes unless customers have explicitly opted in to receive them via consent language displayed in your app's UI,
  and you provide a method in your app for a user to opt out from receiving such messages"
  ([App Review Guidelines](https://developer.apple.com/app-store/review/guidelines/), last updated 2026-10-16). The OS
  permission alone is not marketing consent on iOS.
- **Android:** from Android 13, `POST_NOTIFICATIONS` is a runtime permission; on a fresh install "your app's
  notifications are off by default" and the app "must wait to send notifications until after you request the new
  permission and the user grants that permission"
  ([Android docs](https://developer.android.com/develop/ui/views/notifications/notification-permission)).
- **Google Play:** a primary Play policy line requiring marketing-push opt-in was not found; the Spam and system
  functionality policies were seen only through search summaries (**unverified**,
  [Developer Program Policy](https://support.google.com/googleplay/android-developer/answer/18258653?hl=en)).
- **Dead tokens.** APNs answers `410` with reason `Unregistered`: "The device token is inactive for the specified topic.
  There is no need to send further pushes to the same device token"
  ([APNs responses](https://developer.apple.com/documentation/usernotifications/handling-notification-responses-from-apns)).
  FCM answers `UNREGISTERED (HTTP error code = 404)`: "the token used is no longer valid and a new one must be used"
  ([FCM ErrorCode](https://firebase.google.com/docs/reference/fcm/rest/v1/ErrorCode)).

## 8. Web push [spec, technical]

- The Push API is a "powerful feature" gated by the `push` permission; `userVisibleOnly` promises each push shows a
  notification ([W3C Push API](https://www.w3.org/TR/push-api/), 3.6).
- Safari on iOS/iPadOS: a Home Screen web app "can request permission to receive push notifications as long as that
  request is in response to direct user interaction — such as tapping on a 'subscribe' button"
  ([WebKit blog, 2023-02-16](https://webkit.org/blog/13878/web-push-for-web-apps-on-ios-and-ipados/)).
- A push service "MUST return a 404 (Not Found) status code if an application server attempts to send a push message
  to an expired push message subscription" ([RFC 8030](https://www.rfc-editor.org/rfc/rfc8030), section 7.3). That
  push services also answer `410 Gone` for an unsubscribed endpoint is common practice but **unverified** here (RFC 8030
  uses 410 only for receipts, section 6.2). The subscription is the Destination; the permission prompt is the consent
  event.

## 9. Proof of consent: what a record must hold

The union of what the sources ask for, against what ADR 0013's `marketing.confirmed` Event carries today (when, how,
provenance, IP):

| Field                                                     | Asked for by                                                                                    | In `marketing.confirmed` today |
| --------------------------------------------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------ |
| Timestamp                                                 | CTIA 5.1.2; GDPR 7(1) implicitly                                                                | yes                            |
| Destination (the number, chat id, subscription)           | CTIA; TCPA (f)(9) "the telephone number"                                                        | via the gate row key           |
| Medium / method (form, keyword, `/start`, in-app prompt)  | CTIA "consent acquisition medium"                                                               | yes (provenance)               |
| IP address                                                | CTIA                                                                                            | yes                            |
| The exact consent language and action shown               | CTIA "capture of experience"; TCPA (f)(9)(i) disclosures; 152-FZ art. 9(1)                      | **no**                         |
| Who is consenting to (seller / brand / Workspace)         | TCPA "authorizes the seller"; FAS "конкретного рекламораспространителя"; WhatsApp business name | implicit (Workspace)           |
| Purpose or campaign (marketing vs informational category) | CTIA "specific campaign"; WhatsApp per-category opt-in; FCC 26-67 categories                    | **no**                         |
| Identity of the consenting person, uniquely identifying   | CTIA; 126-FZ 44.1(1); FAS 2019 (a web form alone is inadequate)                                 | partly (Contact)               |
| Signature (TCPA written consent: E-SIGN-valid)            | TCPA (f)(9)(ii)                                                                                 | **no**                         |
| Opt-out records, kept 5 years (US DNC)                    | 64.1200(d)(6); CTIA 5.1.5                                                                       | Unsubscribe survives erasure   |

For Russia, "uniquely identifies the subscriber" points at a confirming action from the Destination itself: an SMS
reply or a one-time code, the SMS analogue of email double opt-in.

## 10. Does an opt-out cover all messages from the business?

- **US SMS [law]:** yes for marketing. From 2026-11-25 a revocation in reply to marketing revokes "all future calls or
  text messages containing an advertisement or [that] constitute telemarketing from that caller"; an informational
  revocation may be read per category (1.2). A DNC request covers the entity and expected affiliates (1.4).
- **US SMS [policy]:** CTIA scopes opt-in and the confirmation "per campaign" (2.1); the provider's block is per number
  or Messaging Service (2.3, unverified). The legal scope is wider than both.
- **EU [law]:** the GDPR objection ends processing "for such purposes", i.e. all direct marketing by that controller
  (3.3).
- **Russia [law]:** 38-FZ stops all ads "в адрес лица" from that distributor; 152-FZ stops all direct-marketing
  processing; the operator-level refusal stops every рассылка from every sender (4.1–4.3).
- **WhatsApp [policy]:** a user's stop covers all marketing templates of the business; the policy demands honoring any
  opt-out request "on or off WhatsApp" (5).
- **Telegram, push:** the block or revoked permission silences the whole bot, app or subscription; the platform makes
  it total.

## Implications for the channel model

What Send-eligibility must enforce, as findings for the grilling tickets, not decisions:

1. **The positive consent layer is mandatory on every new channel.** Today Confirmation is optional (workspace toggle,
   default off). For SMS (US policy, EU, Russia) and WhatsApp (policy) a marketing send without a recorded opt-in for
   (channel, destination) is unlawful or against policy. On Telegram and push the Destination cannot exist without an
   opt-in (`/start`, permission, subscription), so creating the Destination is the consent record; iOS marketing push
   also needs in-app consent. Only email keeps "absence of an opt-out means subscribed" as a default.
2. **Soft opt-in exists only in the EU, only for "electronic mail" (incl. SMS)**, only for the seller's own similar
   products after a sale, and only with an opt-out at collection and in every message. If modelled, it is a Confirmation
   provenance, not a bypass; US carriers (CTIA) and Russian law do not accept it.
3. **STOP is broad.** A keyword reply on SMS, a WhatsApp marketing stop or a Telegram block should map to the
   channel-wide `everything` scope for that Workspace, not to the one Automation that sent the message (US from
   2026-11-25, 38-FZ, 152-FZ, GDPR art. 21, WhatsApp). The per-Sending-source opt-out can survive as an extra, finer
   choice offered by link. The record must be keyed by Workspace and Destination, not by the provider's sender number.
4. **Opt-outs arrive from outside the channel.** US (a)(11) presumes revocation from an email or voicemail unless the
   sender designated exclusive means and discloses them in every text; WhatsApp requires honoring requests "on or off
   WhatsApp". The model needs an operator or API action to record an Unsubscribe for channel X received by any route.
5. **Provider signals split in two.** User choice becomes an Unsubscribe: Twilio `21610`, an inbound STOP word,
   WhatsApp `user_preferences: stop` or `131050`, Telegram `my_chat_member` → blocked. A dead Destination becomes a
   Suppression: APNs `410 Unregistered`, FCM `404 UNREGISTERED`, Web Push `404` (and `410`, unverified).
   START/UNSTOP, WhatsApp `resume` and a Telegram unblock lift the Unsubscribe; whether an unblock counts as renewed
   marketing consent under EU or Russian law is open. The Russian operator-level refusal surfaces only as a delivery
   failure; whether it maps to Suppression or Unsubscribe depends on what the aggregator reports (not researched).
   Interaction with ADR 0013: if STOP writes an `everything` Unsubscribe, it also invalidates the Confirmation gate
   row, so START/UNSTOP alone lifts the Unsubscribe but leaves the Contact ineligible while the positive layer is
   mandatory. A START sent from the Destination is itself a consenting action and could write the Confirmation too.
6. **Two messages must pass an Unsubscribed gate.** The single opt-out confirmation after STOP (FCC (a)(12), presumed
   fine within 5 minutes, no marketing; CTIA "one final … confirmation") and the CTIA opt-in confirmation sent before
   the first recurring message. Both are non-marketing sends on SMS, which strains the map's "Transactional stays
   email-only" decision. Flagged, not resolved.
7. **Check eligibility at dispatch.** Russian law says "немедленно"; the operator refusal bites the next calendar day;
   the US ceiling is ten business days (a seven-day ceiling is proposed). A Broadcast that resolves its audience at
   schedule time can violate these; the check belongs at send time.
8. **Quiet hours need the Contact's time zone.** The only legal window found is the US 8:00–21:00 for unconsented
   solicitations (and Florida's 8:00–20:00 for calls); none in the EU or Russia at the level researched. Consented
   marketing has no federal US window, but the map already lists a send window as deferred.
9. **Consent evidence grows.** Add the consent text shown, the purpose/category, the brand, and (for US written
   consent) the signature method; Russia favours a confirming action from the Destination (SMS reply or code). The
   marketing consent must be a separate checkbox or document (152-FZ art. 9(1)).
10. **Destination keys differ per channel**: phone number (SMS), `wa_id` or a business-scoped user ID (WhatsApp, which
    may hide the number), chat id (Telegram), device token (mobile push), subscription endpoint (web push).
11. **Workspace-level, not message-level:** 10DLC brand/campaign registration (US), the per-operator contract
    (126-FZ 44.1(2)), the foreign-messenger ban for listed Russian organizations (41-FZ art. 15). These belong to the
    Integration or the Workspace's own compliance, not to Send-eligibility.

## Open questions

1. Is the per-channel positive layer a fixed rule ("SMS and WhatsApp always require a Confirmation") or a per-channel
   policy the Workspace may relax for jurisdictions that allow it (US informational texts, EU soft opt-in)?
2. Should "STOP" map to channel-wide `everything`, and the per-source unsubscribe exist on SMS at all, given 160
   characters and the FCC's per-category rule only for informational messages?
3. Do the opt-out and opt-in confirmation texts become a third message class (neither marketing nor Transactional)?
4. 38-FZ art. 18(2) on "автоматической рассылки": how FAS and the courts apply it to consented automated campaigns.
5. Is a web or app push "electronic mail" under ePrivacy art. 13 in any member state's practice?
6. Twilio's exact opt-out scope, and what Russian aggregators return when the operator-level refusal blocks a send.

**Caveat.** This is research from primary sources, not legal advice. National transpositions of ePrivacy art. 13,
US state "mini-TCPA" laws beyond Florida, Canada (CASL), the UK after the Data (Use and Access) Act 2025, and the
enforcement practice of FAS and Roskomnadzor were not covered. A Workspace sending marketing on these channels should
have counsel review its consent wording, the evidence it keeps and its opt-out handling.
