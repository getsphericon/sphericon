---
status: accepted
---

# Identify writes a Contact's core attributes

A Contact's core attributes are the ones sphericon itself reads (its time zone, its country) and the
ones most marketing platforms model as standard (Segment's reserved traits, Braze's standard
attributes): first and last name, time zone, country, locale, birthday, city and avatar. Everything
else is a Custom field (ADR 0006). Before this, Identify put every trait into Custom fields, so a
tracker sending `firstName` or `country` made a Custom field that shadowed the core attribute of the
same name: the duplicate ADR 0006 exists to prevent. The external API's upsert filled a core
attribute only when the Contact had none.

## Decisions

- **A trait named after a core attribute writes that attribute.** The names are the contract's
  camelCase ones (`firstName`, `timeZone`, `country`, `locale`, `birthday`, `city`, `avatar`), the
  names Segment reserves where it has one. No Custom field may take one of those keys, from any
  source. A `first_name` trait is an ordinary Custom field: one spelling is reserved, not every
  spelling someone might type.
- **An asserted value overwrites.** Identify and the external API's upsert both replace a core
  attribute with the value they are given, as Segment, Braze and HubSpot do; `null` clears it and an
  absent key keeps it, the JSON Merge Patch rule of an update. One rule for both doors: a name that
  changes through the tracker and not through the API could not be explained.
- **Alias keys are not overwritten.** `subjectId`, `email` and `phone` still only fill a Contact that
  has none (ADR 0002): they resolve identity and are unique per Workspace, so overwriting them would
  move a Contact's identity or collide with another Contact's. As traits they are ignored.
- **An invalid value is dropped, not refused.** A trait that is no valid value of its attribute (an
  unknown time zone, `USA` as a country, a photo URL that is not https) is not stored, and the rest of
  the Identify succeeds: it comes from a visitor's browser, and refusing it would lose the binding of
  the Visitor to its Contact over one bad value. It never becomes a Custom field either.
- **What the browser infers only fills.** The tracker sends the browser's time zone and language
  with an Identify. They set the Contact's time zone and locale only while it has none, and a trait
  of the same attribute wins: the browser is a guess, the customer's trait an assertion, and a laptop
  on a trip must not reset the time zone an operator set.
- **A photo is shown from its own URL.** The app loads the `https` URL in the operator's browser, with
  the Contact's initials while it has none or it fails to load. No proxy hides the operator's address
  from the image host: an image request carries little, and a proxy is infrastructure of its own.

## Considered options

- **Fill only, as the upsert did.** Rejected: every platform the customers come from overwrites, and
  a fill-only attribute can never be corrected from the source system.
- **Reserve both camelCase and snake_case.** Rejected as more rules than the problem needs; the
  contract has one spelling.
