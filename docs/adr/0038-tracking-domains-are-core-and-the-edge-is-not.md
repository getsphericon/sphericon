---
status: accepted
---

# Tracking domains are a core feature; the edge that terminates their TLS is not

Customers want the links in their mail on their own hostname (a CNAME). Links are built from the one
`SPHERICON_APP_URL` today. ADR 0028 left the edge open: its ALB holds 25 certificates by default, a
quota AWS lists as adjustable with no stated maximum, so the quota is not what decides this. What
decides it is that an ALB certificate needs a DNS validation record in the customer's zone as well
as the CNAME, while a CloudFront multi-tenant distribution validates over HTTP and renews on its own.

## Decisions

- **The model is in the core, ungated.** A Workspace has at most one Tracking domain (one per
  Workspace, as in Drip). Limiting it by plan belongs to the billing plane, not to a license feature.
- **Ownership is a live DNS check.** The customer creates a CNAME to the Instance's edge host
  (`SPHERICON_TRACKING_CNAME_TARGET`, which the core only compares with the DNS answer). A domain
  belongs to exactly one Workspace on the whole Instance. A periodic job re-checks it: `pending`,
  `verified`, `failed`. Nothing is served and no certificate is requested for a domain that is not
  verified; that is the abuse control.
- **The customer's host serves link paths only.** Click, open, unsubscribe and confirm; everything
  else (the SPA, `/api`, `/site`, `/mcp`, sign-in) is a 404, and a link token that belongs to
  another Workspace is a 404 too.
- **Links degrade, they do not break.** While the domain is not verified, new mail builds links from
  the Instance's own host; links already sent keep working while the CNAME points at us and always
  work on the Instance's host (ADR 0036). The Owner is told when a domain fails.
- **Only link paths move.** The first-party tracker (`/t.js`, `/collect`) is a separate decision.
- **The edge is outside the core.** On the hosted service a closed service in `getsphericon/platform`
  reads the verified domains through a new `domains:read` scope of the service surface (like the
  Workspace feed) and creates and removes CloudFront multi-tenant distribution tenants with ACM
  certificates; the ALB keeps serving the platform's own four hosts. A Self-hoster runs their own
  proxy, and the core serves an `ask` endpoint (answers 200 only for a verified domain) so Caddy's
  on-demand TLS needs no logic of theirs.

## Considered options

Caddy on-demand TLS as the hosted edge: one mechanism for every deployment, but the hosted service
would run certificate storage and live inside Let's Encrypt's per-domain weekly limits, where
issuing for arbitrary customer names can block renewals of its own certificates. ALB with ACM per
domain: cheapest for a few domains, but a second DNS record for the customer and a certificate quota
to watch. Neither was chosen for the hosted edge.

## Consequences

CloudFront tenant pricing is by tenant and the figures found disagree: confirm them on AWS's pricing
page before the platform ticket is built. Changing or removing a customer's domain leaves links
already in inboxes pointing at a host that no longer reaches us: the Instance's own host keeps
serving them, the customer's does not, and the documentation says so.
