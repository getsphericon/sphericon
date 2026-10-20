---
status: accepted
amended-by: 0049 (the deleted_at column is discarded_at)
---

# Workspaces have no slug and are deleted softly

A Workspace was addressed by a slug derived from its name, immutable and globally unique, so the
name and the address were coupled: a name could not change freely and a taken name got a silent
`-2`. A Workspace has no public page or subdomain, so the slug bought only a readable URL. Prior
art for marketing tools (Drip addresses an account by its id) agrees.

## Decisions

- **A Workspace has a name and an id, no slug.** The name is free text, not unique, changed at will
  by an Owner or Admin. The id is a short opaque value (8 base62 characters) assigned at creation,
  immutable, and the only address: `/w/{id}`, `/workspaces/{id}/...`. The internal `int64` key
  stays internal.
- **Any Workspace is created, renamed and deleted from the UI.** A verified User creates as many as
  they like and becomes Owner. The first Workspace, made when the email is verified, is named
  "Personal" and is otherwise an ordinary Workspace: no kind, no special rules.
- **Deletion is soft.** Only an Owner deletes, confirming by typing the name. For 30 days the
  Workspace is frozen (a Hold, as in ADR 0007) and hidden, and restorable; then a river job erases
  its data (heavy work is a job, never a migration, ADR 0033). The last Workspace may be deleted:
  a User with none is sent to create one.

## Considered Options

- **Keep an editable slug**: a readable URL, but it needs a format, a reserved-word list, collision
  rules and redirects for old links (Routable redirects for 60 days), all for an address nobody
  outside the Workspace sees; rejected.
- **Unique names instead of slugs**: brings the slug's problems back (first claimer owns "Acme");
  rejected, Users tell duplicates apart themselves.
- **A `personal` kind with a delete block**: protects against an empty account, but the empty state
  is cheap (the create screen) and the rule is one more thing to explain; rejected.
- **Hard delete on confirmation** (Notion): simplest, but Contacts and consent facts are legally
  significant and a mis-click must not erase them; rejected.
