---
status: accepted
build: [1102]
---

# A web push Binding reaches a Contact only by proof, and its browser's reports keep it current

> Amends [[0171-an-identify-through-the-collect-key-is-a-claim-until-the-customers-server-proves-it]]
> (a proven Identify announces the Bindings it hands over on every Channel, and the tracker's push
> reports carry the Identity token), [[0077-consent-on-the-new-channels]] (an explicit subscribe is
> web push's reversal in the Channel), [[0093-the-tracker-waits-for-storage-consent-when-asked]]
> (the push calls under Tracker consent) and
> [[0121-a-preference-page-per-destination-and-only-the-person-lifts-a-refusal]] (what the page
> shows for a web push Destination).

## Context

A browser's push subscription belongs to one browser profile on one origin, not to a person. The
tracker reports it through the public collect key, together with the Visitor id the page's cookie
holds. The first web push spec (#1102) made the Binding the Visitor's and let any Identify reach it
through the Visitor's Contact, the latest report winning. ADR 0171 has since made an Identify through
the collect key a claim: a claim may bind an anonymous Visitor to any Contact it names, so a Binding
reached through the Visitor would let anyone who claims another person's email receive that person's
personalised pushes in their own browser. ADR 0171 settled Telegram: a Visitor's Binding reaches no
Contact until a proven Identify hands it over. Web push needs the same rule, and two things Telegram
does not have: the browser reports its subscription again on later page views, and nothing but the
reporting page knows who sits at the browser now.

Push platforms split the same way. A platform that messages a browser without a person (an anonymous
subscription) sends to whoever allowed it; a platform that messages people ties a browser to a
person only through the customer's sign-in, and the ones that guard it check a signature from the
customer's server before they do. sphericon sends to Contacts (ADR 0075): every Segment, Condition,
figure and Send-eligibility check counts Contacts.

## Decisions

- **A push report carries the Identity token** the tracker holds from the page's latest Identify,
  when it holds one, as the Telegram start link request does (ADR 0171). A token that does not
  verify refuses the report; it is never read as a report without one.
- **A report whose token verifies gives the Binding to the Contact it proves**, made or moved, and
  records `web_push.bound` for it and `web_push.unbound` for a Contact it takes the Binding from.
  The same Contact reporting again records nothing.
- **A report without a token:**
  - makes a Binding owned by the reporting Visitor when the endpoint has none;
  - leaves a Binding that is the reporting Visitor's Contact's where it is, and only refreshes its
    keys;
  - moves a Contact's Binding to the reporting Visitor when that Visitor is anonymous or another
    Contact's, recording `web_push.unbound` for the Contact. A browser whose site reset the Visitor
    at a sign-out (ADR 0171) stops showing the previous person's pushes on its next page view;
  - moves another Visitor's Binding to the reporting Visitor, recording nothing.
- **A proven Identify hands the Visitor's Bindings on every Channel to the Contact it proves**, and
  records `<channel>.bound` for each: `telegram.bound` and `web_push.bound`. A claim, a Form submit
  and a Link identity token hand over nothing (ADR 0171).
- **A Visitor's Binding reaches nobody.** No Broadcast, Automation, Condition or figure counts a
  browser that no Contact owns. Web push therefore reaches the visitors a site has proven, its
  signed-in customers; a site without the Identity secret collects Bindings that reach nobody, and
  the web push Integration's setup says so and leads to the Identity secret.
- **An explicit subscribe lifts a web push opt-out on its Destination.** `subscribeToPush()` follows
  a click of the visitor, so it is the person asking to receive again, as a Telegram `/start` is
  (ADR 0077). It records `web_push.resume_observed`, a consent observation ordered with the others by
  when the Instance received it (ADR 0154), so a delayed or repeated report never undoes a later
  opt-out. A report a page view makes never lifts one.
- **Under Tracker consent (ADR 0093)**, while consent is not `granted`, `subscribeToPush()` resolves
  `unavailable` without asking the browser, since no Visitor exists to own the Binding; `denied`
  stops the page-view reports and leaves the browser's subscription alone, since the visitor allowed
  it in the browser.
- **On the Preference page (ADR 0121)** a web push Destination shows its **Channel stop** and that
  the site's own button undoes it, as SMS and Telegram show theirs. No `preference` message is ever
  pushed: the way back is the site's button.

## Consequences

- The collect operation that reports a subscription takes `identityToken`, and the tracker sends the
  token it holds with every push report.
- The hand-over of a proven Identify is Channel-generic and announces each Binding on its Channel.
- A web push Integration's setup includes the Identity secret, and its page says that subscribers
  of an unproven visitor reach nobody until the site proves who they are.
- `web_push.resume_observed`, `web_push.unsubscribed` and `web_push.opt_out_recorded` are evidentiary
  System events (ADR 0019), as their SMS and Telegram counterparts are.

## Considered options

- **Sending to a Visitor's Binding** (an anonymous subscriber). Reaches a blog's readers, but a
  Recipient is a Contact everywhere else (ADR 0075), and a second kind of Recipient would split every
  Segment, figure and eligibility check.
- **Letting a claim hand a Binding over.** The leak ADR 0171 closes: a claim of someone else's email
  would route their pushes to the claimer's browser.
- **The Binding always the Visitor's, reaching the Visitor's Contact** (the first spec). A claim binds
  an anonymous Visitor to any Contact, so it is the same leak.
- **Keeping a Contact's Binding on every report without a token.** A shared browser would keep
  showing the previous person's pushes after the site reset its Visitor at a sign-out.
- **Moving a Contact's Binding on every report without a token.** A page view before the page's own
  Identify would take the browser from its proven Contact and give it back a moment later, firing
  `web_push.unbound` and `web_push.bound` and the Automations they trigger each time.
