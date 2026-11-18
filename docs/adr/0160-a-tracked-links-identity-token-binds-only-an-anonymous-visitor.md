---
status: accepted
build: [1461]
---

# A tracked link's identity token binds only an anonymous Visitor

> **Amended by [[0171-an-identify-through-the-collect-key-is-a-claim-until-the-customers-server-proves-it]]:**
> an Identify without an Identity token is a claim and moves no Visitor that belongs to another
> Contact, so a wrong binding from a forwarded message is corrected only by a proven Identify. A
> Link identity token hands no Binding to the Contact.

## Context

A person who clicks a link in a marketing message lands on the Workspace's site as an anonymous Visitor:
the browsing that follows is not their Contact's until they sign in or fill a form. Marketing platforms
add an identifier of the recipient to the links of their messages so that the site's tracker recognises
them on arrival. The risk is a forwarded message: whoever clicks the forwarded link is taken for the
recipient.

A form submit already faces the same risk, since anyone can type someone else's email, and binds only a
Visitor that belongs to no Contact, never moving one from another Contact (ADR 0086). A message is
tracked only when its Destination's Engagement tracking consent allows it (ADR 0120), and the tracker runs
only with Tracker consent.

## Decisions

- **A Link identity token is added to the links of a tracked message when the Workspace turns it on.**
  When the click of a tracked link (an email's rewritten link, a Short tracked link on SMS or Telegram)
  redirects to its destination, a token naming the Contact is added to that URL. It is a signed link
  token of its own kind, as the click links' own tokens are, naming the Workspace and the Contact by
  their public ids (ADR 0142) and no message, so it carries no personal data. No other link token is
  read as one: an unsubscribe link, which names a Contact too and never expires, binds nothing.
- **Only on a click that is recorded.** The token is added exactly when the click is recorded
  (ADR 0120): the message is tracked and its Destination's consent is to track now. A click that
  records nothing (an untracked message, its tracking withdrawn, a refusal since the send) redirects
  without it. Whether a machine made the click does not matter: a scanner is kept out on the site
  (below).
- **Only to the Workspace's own sites, over `https`.** The Workspace lists the hostnames where its
  tracker runs, and only an `https` destination on one of them gets the token; a link to anyone
  else's site never carries it, and neither does a plain `http` one, where it would travel in clear.
- **Off until the Workspace turns it on and lists its domains.** It changes how identities are joined,
  which a Workspace decides on purpose.
- **It binds only an anonymous Visitor**, as a claim binds (ADR 0171). The tracker, with Tracker
  consent, reads the token and binds the Visitor to the Contact, stitching its anonymous Events onto
  it and writing no attribute. A Visitor that already belongs to a Contact stays where it is, as a
  form submit leaves it (ADR 0086). It hands the Contact none of the Visitor's Bindings: a Telegram
  chat started from the Visitor stays out of the Contact's reach.
- **A wrong binding is corrected only by a proven Identify.** A forwarded message binds the stranger
  who clicks it; a later Identify without an Identity token, a form submit or another token leaves
  that Visitor where it is (ADR 0171), and a proven Identify moves it. Where the Workspace signs no
  Identity tokens, the binding stays, bounded by the 7 days and the listed sites.
- **It is presented on the person's first interaction with the page.** The tracker removes the token
  from the address bar on load, so a copied URL does not carry it on, keeps it in memory for that
  page only, and sends it on the first pointer press, key press or touch, before the page is left.
  A scanner that loads a link and runs its script makes none of them, so it binds no one; the page
  views before the interaction are stitched once it binds, and a person who leaves without one is
  not identified. A request whose User-Agent names a known crawler is ignored as well.
- **It lasts 7 days from the message's send**, and a token past that is ignored.

## Consequences

- The Workspace settings gain the switch and the list of hostnames; the click redirect appends the
  token when the click is recorded; the collect API takes the token with the Visitor id and answers
  the same whatever it binds, so a token cannot be probed; the tracker strips it on load and presents
  it on the first interaction.
- Glossary: **Link identity token**.

## Rejected

- **On by default**: a Workspace would join identities it never chose to.
- **Moving a Visitor that belongs to another Contact**: a forwarded message would move a known person's
  browsing to a stranger.
- **A token without expiry**: a message read months later, or forwarded around, keeps binding strangers.
- **Sealing the whole token under the Instance's id key**: that key seals one primary key per block
  and never rotates; a token naming a Contact until a date is a signed link token, whose secret
  rotates, with its ids in their public form.
- **A later claim moving a Visitor a token bound**: the Visitor would have to remember how it was
  bound, and a claim typed on the recipient's own browser (another person's email on a shared
  computer) would move them to a stranger, which ADR 0171 closes.
- **No token on a click judged a machine's**: the judgment (ADR 0053) is a partial signal kept for the
  rates, and it counts every click from an iCloud Private Relay range as a machine's, which is where
  a person's click opened in Safari with Private Relay on comes from. The scanner is kept out where
  it shows: it never interacts with the page.
- **Presenting the token on load**: a scanner that runs the page's script would bind its own Visitor,
  whose page views then read as the recipient's though nobody clicked.
