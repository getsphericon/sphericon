---
status: accepted
---

# Tracking domains are a core feature; the edge that terminates their TLS is not

Customers want the links in their mail on their own hostname (a CNAME). Links are built from the one
`SPHERICON_APP_URL` today. The SaaS deployment (ADR 0002 in the platform repository) left the edge open: its ALB holds 25 certificates by default, a
quota AWS lists as adjustable with no stated maximum, so the quota is not what decides this. What
decides it is that an ALB certificate needs a DNS validation record in the customer's zone as well
as the CNAME, while a CloudFront multi-tenant distribution validates over HTTP and renews on its own.

## Decisions

- **A Tracking domain belongs to a Sending domain.** It is the fixed name `track.<sending domain>`,
  listed among the DNS records of the Sending domain (ADR 0010), and mail sent from that domain builds
  its links on it, so the link host aligns with the From domain. There is no separate entity and no
  choice of label. Drip's help center documents custom sending domains and shows no branded link
  domain, so this follows its unit of branding (the Sending domain), not a feature of it. The core
  has no license gate; a plan limit belongs to the billing plane.
- **Two states, because a certificate takes time.** `dns_verified`: a live DNS check finds the CNAME
  pointing at the Instance's edge host (`SPHERICON_TRACKING_CNAME_TARGET`, which the core only compares
  with the answer); the edge may now provision and the `ask` endpoint answers 200. `serving`: an HTTPS
  request through the SSRF-guarded client to a well-known path on the domain succeeds. New mail uses the
  domain only when it is `serving`, so no message carries a link that fails TLS. A periodic job rechecks
  both and a regression falls back to the Instance's host. The core holds no edge-specific code.
- **A host belongs to one Workspace.** A Sending domain is unique only per Workspace, so the tracking
  host is unique across the Instance among rows in `dns_verified` or `serving`; unverified claims do not
  block anyone, and only whoever controls the DNS can verify.
- **The customer's host serves link paths only.** Click, open and the one-click unsubscribe POST. The
  page links (unsubscribe, confirm) redirect, with an absolute URL and the same token, to the
  Instance's own host, where the SPA renders them; everything else (the SPA, `/api`, `/site`, `/mcp`,
  sign-in) is a 404, and a token that belongs to another Workspace is a 404 too.
- **Links degrade, they do not break.** Links already sent keep working on the Instance's host (ADR 0036) and on the customer's host while the CNAME points at us. The Owner is told when a domain fails.
- **The core reads the customer's host from a configured header**, set by the edge and trusted only on
  requests that come from it; how a given edge sets it is the edge's concern. The `ask` endpoint is on
  the internal listener only, so it cannot be used to list customers' domains.
- **The domains read is a change log, so a removal is reported.** A state filter alone would let a
  host that leaves `dns_verified` and `serving` vanish without a word, and the edge could never tear it
  down. The read pages an append-only log (`GET /service/domains/changes`) whose entries say a host
  became `dns_verified`, became `serving` or was `removed` (its record stopped resolving, its Sending
  domain was deleted or renamed). An ent hook on the Sending domain
  (`internal/trackinghostlog`, no database trigger: the schema is plain tables) writes the entries in
  the transaction of the change, so a write from any path (the recheck job, the console, a delete) is
  logged, and an advisory lock taken in that transaction makes positions commit in the order they were taken. A caller applies entries in
  order and starts from an empty cursor to rebuild the set.
- **Only link paths move.** The first-party tracker (`/t.js`, `/collect`) is a separate decision.
- **The edge is outside the core.** On the hosted service a closed service in `getsphericon/platform`
  reads the domains that are `dns_verified` through a new `domains:read` scope of the service surface
  (like the Workspace feed: an opaque cursor, at-least-once) and creates and removes CloudFront multi-tenant distribution tenants with
  ACM certificates; the ALB keeps serving the platform's own four hosts. A Self-hoster runs their own
  proxy, and the core's `ask` endpoint lets Caddy's on-demand TLS decide with no logic of theirs.

## Considered options

Caddy on-demand TLS as the hosted edge: one mechanism for every deployment, but the hosted service
would run certificate storage and live inside Let's Encrypt's per-domain weekly limits, where issuing
for arbitrary customer names can block renewals of its own certificates. ALB with ACM per domain:
cheapest for a few domains, but a second DNS record for the customer and a certificate quota to
watch. One Tracking domain per Workspace, independent of the Sending domain: a simpler model, but the
link host would not align with the From domain of a Workspace that sends from several. None was
chosen.

## Consequences

CloudFront tenant pricing is by tenant and the figures found disagree: confirm them on AWS's pricing
page before the platform ticket is built, and confirm how a tenant passes the customer's host to the
origin without needing the origin's certificate to cover it. Removing a Sending domain or its CNAME
leaves links already in inboxes pointing at a host that no longer reaches us: the Instance's own
host keeps serving them, the customer's does not, and the documentation says so.
