# Sign-up consent prior art: terms, privacy notice, marketing consent, evidence

Researched 2026-10-21 from primary sources: statute text from the EU Publications Office (Cellar, the same text
EUR-Lex serves), EDPB guidelines PDFs, the CJEU judgment, US appellate opinions (Ninth Circuit's own site, Second
Circuit text from the Caselaw Access Project), ICO and FTC pages, and the providers' and projects' own docs. No blogs,
no law-firm summaries. Anything not confirmed in a primary source is marked **unverified**. Limits: CNIL's pages
returned HTTP 503 during the research, so no CNIL claim is made; EUR-Lex itself blocks scripted fetches, so
statute quotes below come from `publications.europa.eu/resource/celex/<id>` (identical text). This is research, not
legal advice.

Terms used as in `GLOSSARY.md` and ADR 0048: **Sign-in provider**, **Linked identity**, **User**, **Workspace**,
**Verified email**. "Operator" = whoever runs an Instance (self-hoster, or the project for the hosted offering).

## 0. Where sphericon stands today

- `/register` (email and password) and the OAuth callback (ADR 0048: "A created User has a Verified email, no
  password, its first Workspace and no mail") both create the User and first Workspace with **no terms, privacy or
  marketing step**. `src/pages/auth/login.tsx` and the locale file hold no terms or privacy copy for sign-up
  (`grep` found none); there is no `ent/schema` field for an acceptance.
- The provider callback is a single redirect round trip: the person clicks "Continue with Google", the provider's
  screen opens, the callback creates the records. No page of ours is shown between "click" and "account exists".

## 1. Terms of service acceptance: clickwrap, browsewrap, sign-in-wrap

Contract formation online needs reasonable notice of the terms plus a manifestation of assent. US appellate law
sorts interfaces into types:

- **Clickwrap**: "website users are required to click on an 'I agree' box after being presented with a list of
  terms and conditions"; **browsewrap**: terms "generally posted on the website via a hyperlink at the bottom of the
  screen". Nguyen v. Barnes & Noble, 763 F.3d 1171 (9th Cir. 2014),
  [opinion](https://cdn.ca9.uscourts.gov/datastore/opinions/2014/08/18/12-56628.pdf).
- Browsewrap holding in Nguyen: where terms are available "via a conspicuous hyperlink on every page of the website
  but otherwise provides no notice to users nor prompts them to take any affirmative action to demonstrate assent,
  even close proximity of the hyperlink to relevant buttons users must click on - without more - is insufficient to
  give rise to constructive notice." The court cites "courts' traditional reluctance to enforce browsewrap
  agreements against individual consumers".
- **Sign-in-wrap** (the "By creating an account, you agree to ..." line under the button) sits in between. Meyer v.
  Uber, 868 F.3d 66 (2d Cir. 2017)
  ([text](https://static.case.law/f3d/868/cases/0066-01.json), Caselaw Access Project) upheld it on its facts: the
  screen was uncluttered, showed only the registration fields and buttons, the sentence "By creating an Uber account,
  you agree to the TERMS OF SERVICE & PRIVACY POLICY" sat directly under the register button, the whole screen was
  visible at once, and "although the sentence is in a small font, the dark print contrasts with the bright white
  background". The test: only if the undisputed facts show "reasonably conspicuous notice of the existence of
  contract terms and unambiguous manifestation of assent" is a contract formed, and conspicuousness "is a function of
  the design and content of the relevant interface".
- Berman v. Freedom Financial, 30 F.4th 849 (9th Cir. 2022)
  ([opinion](https://cdn.ca9.uscourts.gov/datastore/opinions/2022/04/05/20-16900.pdf)): the same two-part test
  (conspicuous notice AND an action such as clicking a button that unambiguously manifests assent). It refused to
  enforce: the notice was in small font and not evidently a hyperlink, and clicking a large green "continue" button
  was not assent to the terms. The concurrence calls these "sign-in wrap" agreements, which "tempt fate under
  California law" and sit in "a gray zone in which enforceability requires conspicuous textual notice that
  completing a transaction or registration signifies consent to the site's terms and conditions".
- Chabolla v. ClassPass, 129 F.4th 1147 (9th Cir. 2025)
  ([opinion](https://cdn.ca9.uscourts.gov/datastore/opinions/2025/02/27/23-15999.pdf)): a website that "provides a
  link to the Terms of Use but does not require that the user actually read them" resembles a sign-in-wrap; the
  panel found no reasonably conspicuous notice on the landing page or first screen and no unambiguous assent
  on the rest. A dissent would have enforced it on the three screens taken together.

Takeaways for the question "is 'by signing up you agree' enough?": it **can** be enforceable (Meyer) but only when
design makes it conspicuous and adjacent to the action; it is the legally riskiest of the three patterns and
outcomes are fact-specific and jurisdiction-specific. A checkbox ("I agree to the Terms") is the clickwrap form
courts "routinely uphold" (Meyer, same opinion). All of this is **US contract law**; EU/UK contract formation and
unfair-terms rules differ and were not researched (**unverified** for them). Note the terms-of-service contract is a
different legal matter from data protection: Art. 7(2) GDPR lets a controller present consent inside a larger
written declaration only if the consent request is "clearly distinguishable from the other matters" (section 3).

## 2. Privacy notice (Art. 13/14) versus consent as a legal basis

- **Notice is a duty, not a consent.** Art. 13(1) GDPR: "Where personal data relating to a data subject are
  collected from the data subject, the controller shall, at the time when personal data are obtained, provide the
  data subject with all of the following information", including controller identity and contact details, "the
  purposes of the processing ... as well as the legal basis", recipients and (in 13(2)) retention, rights and the
  right to withdraw consent where consent is the basis
  ([Regulation (EU) 2016/679](https://publications.europa.eu/resource/celex/32016R0679), Art. 13).
  The duty is met by making the information available; no tick is required.
- **Art. 14** covers data not obtained from the data subject: information is due "within a reasonable period after
  obtaining the personal data, but at the latest within one month" (Art. 14(3)(a)), or at first communication.
  For a provider sign-up, the name and email come from the provider's response, but the person triggered the
  transfer, so Art. 13 (collected from the data subject, at the time) is the closer fit; which of 13/14 applies to
  provider-sourced profile data is not settled in the sources read (**unverified**, open question below).
- **Consent is the wrong basis for creating an account.** Art. 6(1)(b): processing "necessary for the performance of
  a contract to which the data subject is party or in order to take steps at the request of the data subject prior
  to entering into a contract". EDPB Guidelines 05/2020 on consent v1.1
  ([PDF](https://www.edpb.europa.eu/system/files/documents/files/file1/edpb_guidelines_202005_consent_en.pdf)),
  para 31: "If a controller seeks to process personal data that are in fact necessary for the performance of a
  contract, then consent is not the appropriate lawful basis" (footnote: "The appropriate lawful basis could then be
  Article 6(1)(b)"). Para 30: necessity is "interpreted strictly" with "a direct and objective link". Para 26: "The
  two lawful bases ... consent and contract cannot be merged and blurred."
- Consequence: email, name, password hash, session data and the Workspace needed to provide the service rest on
  Art. 6(1)(b) (or 6(1)(f) for security logging), with a privacy notice; a consent checkbox for them would be
  both unnecessary and, if refusal blocked sign-up, an Art. 7(4) problem. What genuinely needs consent is
  optional extras: marketing email (section 3), non-essential cookies or analytics on the app itself.
- Withdrawal of a consent-based processing must be "as easy to withdraw as to give" (Art. 7(3)); a contract basis has
  no such right, but Art. 21 objection and Art. 17 erasure still apply.
- Art. 7 and Art. 13 do not require "accepting" a privacy policy. Phrasings such as "I accept the privacy policy" are
  common but legally redundant for GDPR; EDPB para 81 warns that "Blanket acceptance of general terms and conditions
  cannot be seen as a clear affirmative action to consent to the use of personal data".

## 3. Marketing consent: GDPR Art. 7, ePrivacy, PECR, CAN-SPAM

**GDPR consent definition and conditions.**

- Art. 4(11): consent is "any freely given, specific, informed and unambiguous indication of the data subject's wishes
  by which he or she, by a statement or by a clear affirmative action, signifies agreement".
- Art. 7(2): a request inside a wider written declaration "shall be presented in a manner which is clearly
  distinguishable from the other matters, in an intelligible and easily accessible form, using clear and plain
  language".
- Art. 7(4): "utmost account shall be taken of whether, inter alia, the performance of a contract, including the
  provision of a service, is conditional on consent to the processing of personal data that is not necessary for the
  performance of that contract."
- Recital 32: "Silence, pre-ticked boxes or inactivity should not therefore constitute consent." Planet49 (CJEU, C-673/17,
  [judgment](https://publications.europa.eu/resource/celex/62017CJ0673), paras 52, 54, 57, 63): "Only active
  behaviour on the part of the data subject with a view to giving his or her consent may fulfil that requirement";
  consent through a pre-ticked checkbox is "not validly constituted". (Planet49 was a cookie/lottery case under
  Directive 2002/58 art. 5(3) read with GDPR; the reasoning is the general consent definition.)
- EDPB 05/2020: bundling consent into non-negotiable terms is "presumed not to have been freely given" (para 13);
  granularity: data subjects "should be free to choose which purpose they accept, rather than having to consent to a
  bundle of processing purposes" (para 42); "consent cannot be obtained through the same motion as agreeing to a
  contract or accepting general terms and conditions of a service" (para 81); "opt-out constructions" are not allowed
  (same passage: "The GDPR does not allow controllers to offer pre-ticked boxes or opt-out constructions").
- ICO ([how to obtain, record and manage consent](https://ico.org.uk/for-organisations/uk-gdpr-guidance-and-resources/lawful-basis/consent/how-should-we-obtain-record-and-manage-consent/)):
  consent requests "separate from any other information such as general terms and conditions"; "You cannot rely on
  silence, inactivity, pre-ticked boxes, opt-out boxes, default settings"; "a separate opt-in for each" purpose,
  "People should not be forced to agree to all or nothing."

**ePrivacy Directive art. 13** ([Directive 2002/58/EC consolidated](https://publications.europa.eu/resource/celex/02002L0058-20091219)):
art. 13(1): electronic mail "for the purposes of direct marketing may be allowed only in respect of subscribers or
users who have given their prior consent". Art. 13(2) soft opt-in: where contact details were obtained "in the
context of the sale of a product or a service", the same person may use them "for direct marketing of its own
similar products or services provided that customers clearly and distinctly are given the opportunity to object,
free of charge and in an easy manner, to such use ... at the time of their collection and on the occasion of each
message". Art. 13(4) bans disguised sender identity and messages without a valid address for stopping them.
Art. 13(5): paragraphs 1 and 3 apply to natural-person subscribers; legal persons are left to national law.

**Does "own product emails" need consent?** Two separate categories:

1. **Service/transactional mail** (verify link, password reset, security notices such as the ADR 0048 "this was not
   me" notice, invitation, billing or abuse notices). Not direct marketing; needs no marketing consent. The sources
   read do not define the boundary; the FTC page says the law "views these categories narrowly" for CAN-SPAM
   transactional/relationship messages ([FTC guide](https://www.ftc.gov/business-guidance/resources/can-spam-act-compliance-guide-business)).
   A "product tips", "newsletter" or onboarding-drip mail is not obviously service mail (**unverified** where the
   line sits under EU/UK law; the sources read give no definition).
2. **Marketing mail** to the operator's own sign-ups (newsletter, product news, onboarding sequences with promotional
   content): in the EU/UK needs prior consent (art. 13(1)) unless the soft opt-in applies. The ICO states the soft
   opt-in covers someone who "bought (or negotiated to buy) a similar product or service from you in the past", the
   opt-out was offered when details were collected and "in every message", and it "does not apply to prospective
   customers or new contacts" ([ICO electronic mail marketing](https://ico.org.uk/for-organisations/direct-marketing-and-privacy-and-electronic-communications/guide-to-pecr/electronic-and-telephone-marketing/electronic-mail-marketing/)).
   A free sign-up is not obviously a "sale or negotiation for a sale" (**unverified**: the sources read do not say
   whether a free account counts), so the safe design assumes consent is needed.

**UK PECR** reg. 22
([legislation.gov.uk](https://www.legislation.gov.uk/uksi/2003/2426/regulation/22)): same structure (prior consent;
soft opt-in for customers with a free, simple refusal). The UK Data (Use and Access) Act 2025 amended PECR; the ICO
page for it was not retrievable, so any change is **unverified** and not described here.

**US CAN-SPAM** is opt-out, not opt-in: the FTC says "you don't need to get members' consent to send them marketing
emails" (same guide); each message needs clear identification as an ad, a valid physical postal address and a
functioning opt-out, honoured within 10 business days (15 U.S.C. 7704(a)(3)-(5),
[statute](https://www.law.cornell.edu/uscode/text/15/7704)). The sender's opt-out also bars selling or transferring
the address. US states and other countries (Canada's CASL, not researched) may differ.

## 4. Evidence to store

- Art. 7(1): "Where processing is based on consent, the controller shall be able to demonstrate that the data subject
  has consented to processing of his or her personal data." Recital 42 repeats it. The burden is on the controller
  (EDPB para 104).
- EDPB 05/2020 paras 106-108: "The GDPR does not prescribe exactly how this must be done"; the controller "may keep
  a record of consent statements received, so he can show how consent was obtained, when consent was obtained and
  the information provided to the data subject at the time". Online: "retain information on the session in which
  consent was expressed, together with documentation of the consent workflow at the time of the session, and a copy
  of the information that was presented ... It would not be sufficient to merely refer to a correct configuration of
  the respective website." Proof must not itself cause "excessive amounts of additional data processing" (para 106):
  "enough data to show a link to the processing ... shouldn't be collecting any more information than necessary."
  Para 107: keep it while the processing lasts, afterwards "no longer then strictly necessary" for a legal obligation
  or legal claims. Para 110-111: consent does not expire by itself; if processing "change[s] or evolve[s]
  considerably" new consent is needed; refreshing at intervals is a "best practice".
- ICO's record list: who ("name ... or other identifier (eg, online user name, session ID)"), when ("a copy of a dated
  document, or online records that include a timestamp"), what they were told (the statement and privacy information
  in use, "matching version numbers and dates"), how, and whether and when consent was withdrawn.
- **What this implies for fields** (derived, not mandated by any source): subject (user id or address), purpose
  (separate per purpose), decision (given or withdrawn, as an append-only history rather than one overwritten
  flag), timestamp, the document identity shown (version id or content hash of the wording and, for terms, of the
  document), source/channel (register form, provider callback step, settings page, API), and the withdrawal event.
  IP address and user-agent are **not** required by any source read; EDPB para 106 argues against collecting more than
  needed, so they are an operator-level choice, not a default. Courts in section 1 cared about the interface shown
  (screen design), which a version id plus a stored copy of the page text can reconstruct, a raw IP cannot.
- For terms of service (contract, not data-protection consent) Art. 7 does not apply, but the same fields prove
  assent in a dispute (Meyer and Berman turned on what the screen showed and what the user clicked).

## 5. Provider sign-in that creates the account on first login

**What providers collect and do not.**

- Google OIDC (`openid email profile`): `sub` ("unique among all Google Accounts and never reused"), `email` and
  `email_verified`, optional `name`/`picture` (never guaranteed), `hd` for Workspace orgs
  ([Google OpenID Connect docs](https://developers.google.com/identity/openid-connect/openid-connect)). The page says the
  consent screen "describes the information that the user is releasing and the terms that apply"; it does not say the
  user accepts the app's terms or privacy policy. Google requires the app's privacy policy and terms links for external
  production apps: "These links are required for all external production apps" and the privacy policy "must be linked
  from the OAuth consent screen" ([Google Cloud consent-screen help](https://support.google.com/cloud/answer/10311615)).
  So Google displays a link to the app's privacy policy; it does **not** collect the user's acceptance of the app's
  terms or give the app a record of acceptance.
- GitHub: with no scope the OAuth app gets "read-only access to public information (including user profile info,
  repository info, and gists)"; `user:email` "grants read access to a user's email addresses"
  ([scopes](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps)). The
  authorization page lists scopes, shown only the first time; the GitHub page read says nothing about privacy policy or
  terms ([authorizing OAuth apps](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps)).
- Neither provider's consent screen is evidence that the person accepted the app's terms or marketing. Their screen
  is about releasing profile data to the app; it is outside the controller's own record.

**Patterns for acceptance (design space).**

| Pattern                                                      | What happens                                                                                                                                                                                                                                                                                                                | Notes                                                                                                                                                                                                     |
| ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A. Notice text next to the provider button                   | "By continuing with Google you agree to the Terms and Privacy Policy" under the button                                                                                                                                                                                                                                      | Sign-in-wrap. Meyer-style conspicuousness needed; Berman and Chabolla show the risk. Provider's own screen follows, so the notice is shown before the click.                                              |
| B. Checkbox before the provider button                       | Button disabled until "I agree" is ticked                                                                                                                                                                                                                                                                                   | Clickwrap; strongest for terms; extra friction; state must survive the redirect (sphericon's flow cookie already carries `return_to` and an invitation).                                                  |
| C. Interstitial after the callback, before the record exists | Callback verifies the id_token, stores a short-lived signed "pending sign-up" (provider, subject, trusted email, name), redirects to an in-app page: "Create your account" with notice or checkbox, optional unticked marketing checkbox; on submit the User, Linked identity, acceptance records and Workspace are created | Matches the "consent before the record is created" ordering; no User row exists for someone who declines; pending state must be signed, time-limited, single use and hold no more than the proven claims. |
| D. Create, then gate                                         | Account is created at the callback; the next page demands acceptance before anything else (a "required action")                                                                                                                                                                                                             | Simplest to build, but a User row, a Workspace and possibly events exist for someone who declined; need rules for abandoned rows (deletion).                                                              |

Prior art, all from the projects' own docs:

- **GitLab** (self-managed, open core): an admin setting "All users must accept the Terms of Service and Privacy
  Policy to access GitLab"; new users get "a mandatory checkbox ... added to the sign-up form"; existing users "must
  accept the terms on their next GitLab interaction", and if they decline they are signed out; "For each update to the
  terms, a new version is stored" and "GitLab records which version they accepted or declined"; enforcement is an
  admin opt-in ([GitLab terms docs](https://docs.gitlab.com/administration/settings/terms/)). The page does not address
  OAuth sign-in (**unverified**). This is a version-tracked, operator-configured, off-by-default model with a
  gate-after-login (pattern D) path for existing users.
- **Keycloak** (open-source IdP): two switches: require agreement to terms "during registration" (registration flow
  row "Terms and Conditions" set to Required), and a "Terms and Conditions" **required action** so "new users must
  accept the terms and conditions before logging in ... for the first time"; the wording lives in a theme template
  (`terms.ftl`) ([Server admin guide](https://www.keycloak.org/docs/latest/server_admin/index.html)). That is pattern D
  as a generic mechanism. How acceptance is stored and whether it triggers for brokered (social) first logins was not
  confirmed (**unverified**).
- **Auth0** documents a terms checkbox on its signup prompt only as a localization example with the label
  "I agree with the [Terms of Service]" ([docs](https://auth0.com/docs/customize/login-pages/universal-login/customize-signup-and-login-prompts)),
  not as a built-in feature with records.
- What large SaaS products (Notion, Linear, Slack, Vercel and similar) show on their first Google or GitHub login was
  **not verified** from a primary source: their sign-up pages are script-rendered and their ToS pages were not read.
  No claim is made here.

## 6. Open-source and self-hosted: who needs the consent

- Under Art. 4(7) the controller is who "determines the purposes and means" of processing. EDPB Guidelines 07/2020 v2.1
  ([PDF](https://www.edpb.europa.eu/system/files/2023-10/edpb_guidelines_202007_controllerprocessor_final_en.pdf))
  para 40: "essential means" are "traditionally and inherently reserved to the controller", while non-essential means
  such as the choice of software "may be left to the processor"; a body that uses software for its own purposes
  remains the one that decides what is processed and why. The guidelines read do not discuss open-source publishers; the
  inference below is **unverified** as to a software author who never touches the data.
- So an **operator** who self-hosts sphericon decides why User data is processed on their Instance and is the party
  that owes the Art. 13 notice to their Users and needs consent for their own marketing mail. The project (software
  author) is not the controller of a self-hosted Instance on this reading. For the **hosted offering**, the project
  is the operator and controller for its own Users, and is also, for the Contacts a customer Workspace holds, in a
  processor role (not researched here beyond Art. 28 existing).
- Distinguish three audiences: (1) the operator's **Users** (people with accounts: this research); (2) the
  **Contacts** inside a Workspace (the Workspace owner's own consent and consent-proof module already exist: GLOSSARY
  consent proof, `marketing.confirmed`); (3) the operator's own product email to its Users (section 3). Only (1) and
  the last are in scope.
- Cross-border: terms and privacy law differ by Instance jurisdiction and by where Users live (GDPR applies by
  residence of data subjects, not by where the operator sits: Art. 3, not read in this research, **unverified** here).
  Hence the software cannot know which regime applies.
- What the software can offer without shipping legal text: operator-configured **URLs** for terms and privacy policy
  (and maybe a contact address for requests); a **version identifier** for each; an off-by-default requirement; an
  evidence store; a withdrawal path for marketing. What it must not hardcode: any legal wording, any assumption of
  jurisdiction, any claim of compliance, the project's own terms as defaults for operators, or, per `AGENTS.md`,
  anything about plans, pricing or the hosted offering.
- GitLab and Keycloak (section 5) both treat the terms as operator-supplied data with the software providing the
  mechanism, which is the same split.

## Implications for sphericon

Design options, not a decision. Each assumes greenfield (no compat shims); they interact, so a grill should pick one
from each group.

**Group 1: what the operator configures**

- 1a. Two optional Instance settings (terms URL, privacy URL), each with an operator-chosen version label.
  Unset = no terms step at all, today's behavior (self-hosters who need nothing are untouched).
- 1b. As 1a, plus a "marketing consent offered" switch (an unticked, separately labelled checkbox) and the operator's
  own sender address; the software never sends marketing to a User who did not tick it.
- 1c. Wording stored as operator-editable text in the database with versions (heavier; closer to Keycloak's theme file
  or GitLab's in-app editor) instead of URLs only.

**Group 2: registration surfaces**

- 2a. Email and password `/register`: one required checkbox (clickwrap) when terms are configured, plus the optional
  marketing checkbox; the privacy link as plain notice text (no tick needed for the privacy policy).
- 2b. Provider sign-in creates the account only after an in-app step (pattern C): the callback holds a signed pending
  sign-up, the SPA shows the same form as 2a, submit creates User, Linked identity, Workspace and acceptance rows in
  one transaction. A returning Linked identity skips it. Declining creates nothing.
- 2c. Pattern A only (notice under the button), cheaper, weaker on enforceability (Berman, Chabolla).
- 2d. Pattern D (create, then gate on the next request), simplest to build, leaves rows for decliners.

**Group 3: records**

- 3a. An append-only `consent`-style table keyed by User with purpose (`terms`, `privacy_notice_shown`, `marketing`),
  decision, document version, source (`register_form`, `provider_callback`, `settings`), timestamp, and a nullable
  withdrawal timestamp or a withdrawal row. Retention is a separate decision (EDPB para 107: keep only as long
  as necessary).
- 3b. A single nullable `terms_accepted_version`/`terms_accepted_at` on `users`: simpler, but loses history and
  cannot show earlier versions (weaker for Art. 7(1) demonstrability).
- IP and user-agent: off by default or not stored; if stored, only because the operator enables it (EDPB para 106).
- Records survive User deletion only for the minimum retention window; interaction with ADR 0050 (a User deletes
  themself at once) needs deciding.

**Group 4: re-acceptance**

- 4a. A new terms version requires re-acceptance at next sign-in (GitLab's model: signed out if declined);
  4b. notice only, no gate; 4c. gate only if the operator marks the change as material.

**Open questions for a grill**

1. Is the default for a fresh install "no terms step" (open core, self-hosters opt in) while the hosted offering turns
   it on through Instance config? Does any of that touch the "neutral about the hosted offering" rule?
2. Marketing mail to Users: does sphericon ship any at all? If not, the marketing checkbox is only a hook for operators.
   If yes, is it a Workspace-level feature using the existing Contacts/consent machinery or a separate User-level one?
3. Provider sign-up: do we accept the extra page for the "first minute" conversion cost (pattern C) or take
   pattern A and carry the Berman risk?
4. Invitation flow (ADR 0048/0051): an invited person who accepts through a provider or password: is acceptance
   required there too, and does a Join-domain admission (no registration step) need it?
5. Existing Users when an operator first turns the requirement on: gate at next request (GitLab) or not at all?
6. Where do declined provider sign-ups go: nothing stored beyond the 10-minute flow cookie, or an audit trail of
   refusals?
7. Staff and service surfaces (`/staff`, ADR 0026): out of scope, they are not public sign-up?
8. Children: Art. 8 GDPR sets 16 (or 13 to 16 by member state) for consent-based services offered to children; a
   B2B marketing tool probably needs only a terms statement of age, but this was not researched.
9. Art. 13 vs 14 for provider-supplied profile data, and a privacy notice link shown before the provider redirect.
10. Does `email_proven_at` (ADR 0048) interact with consent? A Verified email proves the address, not that the owner
    agreed to marketing; the two facts must stay separate in the schema.

**Caveat.** This is research from primary sources, not legal advice. Jurisdictional differences (national
implementations of ePrivacy art. 13, member-state ages of consent, UK law after the 2025 Act, CASL, US state laws,
EU contract law) are mostly not covered. Anyone shipping a hosted sign-up flow should have counsel review wording, the
retention period and the controller/processor allocation.
