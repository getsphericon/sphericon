---
status: accepted
---

# A Tracking domain also serves the first-party tracker

ADR 0038 left the first-party tracker (`/t.js`, `/collect`) as a separate decision and made the
customer's Tracking host serve link paths only. A customer who installs the snippet on their site
loads the script from and posts events to the Instance's host, a name a blocklist can list by
hostname. The Tracking domain already is a name under the customer's own domain with a CNAME to the
Edge target, a verification, a recheck job and an edge tenant, so the tracker reuses it instead of
adding a second host.

## Decisions

- **The Tracking domain serves the tracker paths as well.** On a verified Tracking host (`dns_verified`
  or `serving`, ADR 0038) the gate also answers `GET /t.js` and `/collect/*`; every other path stays a 404. There is no new entity, no new DNS record for the customer and no new verification: the CNAME the
  customer already points at the Edge target carries both.
- **A customer host accepts only its own Workspace's collect key.** The gate knows the Workspace that
  owns the host (ADR 0038 does the same for link tokens). `/collect` on that host refuses a collect key of
  another Workspace as it refuses a wrong one (401, counted against the failed-authentication limit of
  ADR 0025), so a verified host is never an ingestion path for someone else's key.
- **The Instance's own tracker host stays the default.** `SPHERICON_TRACKER_URL`, new with this ADR (the app origin
  when unset, a trailing slash dropped), is what the Tracking settings page puts in the snippet and the test command. A Workspace with a
  Tracking domain in `serving` also sees a snippet built on that host, with the hint to use the one
  under its own site's registrable domain. Nothing is removed: snippets installed on the Instance's
  host keep working (ADR 0036).
- **It is a core feature with no license gate**, like the Tracking domain. A limit on how many a
  Workspace may add belongs to the platform that hosts the Instance. The core holds no edge-specific
  code, and the edge's route for these paths is the edge's concern: `/t.js` may be cached, `/collect`
  is never cached.
- **The claim is hostname-based blocklists, not cookies.** A script and an endpoint under the site's own
  registrable domain avoid lists that name the Instance's host. Browsers that uncloak a CNAME, and
  cookie limits on responses from such hosts, are not promised away: this ADR does not rely on them, and
  the documentation does not either.
- **First-party needs the same registrable domain.** `track.news.example.com` is same-site with
  `www.example.com`; a Workspace that sends from an unrelated domain gets no benefit from it. The app
  does not know the customer's site, so it shows the snippets and the hint and checks nothing.

## Consequences

The hosted edge must route `GET /t.js` and `POST /collect/*` on a customer host, and its rate limit and
filtering that cover `/collect` on the Instance's tracker host must cover these hosts too. That is a
change in the platform repository, not here. The glossary entry of the Tracking domain, the Tracking
settings page and the tracking guide in the documentation are updated with the implementation.
