---
status: accepted
---

# Display rules: the browser decides when, the server decides whom

An On-site form (ADR 0085) shows on the customer's site by its Display rules. The vendors read for
the map (getsphericon/sphericon#948, `research/on-site-messages-and-forms`) all evaluate triggers in
the browser and select the eligible audience on the server. A Segment's Condition tests one
Contact's fields, Tags and Events on the server (ADR 0029), so an anonymous Visitor is in no
Segment. sphericon has no cart model and no GeoIP. Decided in the display rules ticket
(getsphericon/sphericon#955); the legal targets are the US and the EU.

## Decisions

- **The browser decides when and where; the server decides whom.** The tracker applies the
  triggers and the page rules; the audience is the server's. How the browser learns it is the
  delivery ticket's.
- **Triggers** (popup, slide-in, bar), combined with AND: on load, after N seconds, a scroll depth,
  exit intent on a desktop (none on a phone: the vendors' substitutes are unreliable), N pages seen
  in the visit, an Event the tracker records on the page (`track('added_to_cart')`). Or only by hand:
  a click on an element the author names by a CSS selector, or a call from the site's code. An
  embedded form has no trigger: it shows where its Placement is, when the page and audience rules
  pass.
- **On a phone a popup opens from its teaser** by default, as Drip does, since Google ranks pages
  with intrusive interstitials lower; the author may turn it off.
- **Page rules:** URL include and exclude (equals, contains, starts with, `*`); device; new or
  returning Visitor; the UTM parameters and referrer the visit began with. No geo (it needs a
  licensed GeoIP database, left for later) and no cart rule (no cart model): a cart prompt comes from
  an Event trigger or a call from the site's code, or from a Segment on Events for a known Visitor.
- **The audience** is everyone, anonymous Visitors, known Visitors, in a Segment or not in a
  Segment; an anonymous Visitor is in no Segment. Publish copies the Segment's Condition into the
  form's version, as Schedule does for a Broadcast (ADR 0065): editing a Segment does not change a
  live form, and the Segment reference stays as provenance.
- **A schedule** has a start and an end; a date without an offset is read in the Workspace time
  zone (ADR 0087). Outside it the form does not show and stays active.
- **A form can be shown on purpose.** A parameter on the site's URL shows the live version of a
  given form on any page, whatever its rules, so a carrier's 10DLC reviewer can see an SMS opt-in;
  the form is public anyway. A signed, expiring link from the app shows a draft on the live site.
  Neither records an Event. The parameter is a contract customer sites carry (ADR 0036).
