---
status: accepted
build: [1461]
---

# A tracked link's identity token binds only an anonymous Visitor

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
  redirects to its destination, a signed token naming the Contact is added to that URL. It is opaque,
  carries no personal data and is sealed under the Instance's id key (ADR 0142). An untracked message
  gets none.
- **Only to the Workspace's own sites.** The Workspace lists the domains where its tracker runs, and only
  a destination on one of them gets the token; a link to anyone else's site never carries it.
- **Off until the Workspace turns it on and lists its domains.** It changes how identities are joined,
  which a Workspace decides on purpose.
- **It binds only an anonymous Visitor.** The tracker, with Tracker consent, reads the token and
  identifies the Visitor as the Contact, writing no attribute. A Visitor that already belongs to a Contact
  stays where it is, as a form submit leaves it (ADR 0086). A later Identify by the customer's code moves
  the Visitor as usual (ADR 0002), so a wrong binding from a forwarded message is corrected by the next
  real sign-in.
- **It lasts 7 days from the message's send**, and a token past that is ignored. The tracker removes it
  from the address bar once read, so a copied URL does not carry it on.

## Consequences

- The Workspace settings gain the switch and the list of domains; the click redirect appends the token;
  the tracker reads and strips it and identifies an anonymous Visitor.
- Glossary: **Link identity token**.

## Rejected

- **On by default**: a Workspace would join identities it never chose to.
- **Moving a Visitor that belongs to another Contact**: a forwarded message would move a known person's
  browsing to a stranger.
- **A token without expiry**: a message read months later, or forwarded around, keeps binding strangers.
