---
status: accepted
---

# One attribute concept: typed Custom fields, auto-created — no schemaless traits

Every Contact attribute beyond the core fields is a **Custom field**: a workspace-defined, typed,
named definition. There is no second, schemaless "trait" concept living alongside it. When an
unknown key arrives — from Identify or an Event payload — it is **auto-created** as a Custom field
with an inferred type (declared-by-use), but from that first sight it is a first-class, typed,
renameable definition, not an anonymous key in a bag.

The core fields are the alias keys (subject_id, email, phone) and the core attributes: the ones
sphericon itself reads and the ones most marketing platforms model as standard (first and last name,
time zone, country, locale, birthday, city, avatar). How Identify writes them is ADR 0072.

This rejects the usual CDP two-layer shape (Segment, Customer.io): raw schemaless traits
flowing in from `identify`, plus a curated/typed subset "promoted" into custom fields. That
shape reliably rots — hundreds of attributes, half of them near-duplicate typos
(`firstName` / `first_name` / `fname`) — and leaves the Segment builder unable to offer a
trustworthy field list. Collapsing to one governed concept keeps the developer convenience of
schemaless ingest (you don't pre-declare every key) while guaranteeing the builder always has
a real, typed, named field catalogue to target.

## Considered options

- **Two concepts: freeform traits + promoted custom fields.** Rejected: two parallel attribute
  notions for one job, an explicit "promote" step nobody maintains, and an ungoverned trait
  space that the segment builder can't safely surface. (See [[GLOSSARY.md]] Anti-vocabulary →
  Trait.)
- **Schema-first: reject unknown keys until declared.** Rejected: breaks the tracker/API
  ingest ergonomics — a customer sending `identify(id, {plan})` would silently lose `plan`
  until an admin declared it. Declared-by-use keeps ingest frictionless.

## Consequences

- Ingest must infer a type on first sight of a key and create the Custom field; later values of
  a conflicting type need a coercion/widening policy (implementation detail, not fixed here).
- Segment rules range over core fields + Custom fields uniformly; there is no "trait vs field"
  distinction to expose in the builder.
- The CDP term "trait" is anti-vocabulary; everything is a Custom field. This composes with
  [[0002-unified-contact-identity]] — one Contact, one attribute model, no CDP-side shadow.
