---
status: accepted
---

# A Tracking domain is the Workspace's own, not a Sending domain's

ADR 0038 made the Tracking domain the fixed name `track.<sending domain>`, a record of the Sending
domain, and ADR 0059 let the same host serve the first-party tracker. A Workspace that only tracks
its site (the snippet today, on-site surfaces later) and sends no mail has no Sending domain, so it
could not have its own host; adding a Sending domain to get one means DKIM, SPF and DMARC records it
never uses. Email platforms mostly tie the link domain to the sending domain (SendGrid, Mailgun,
Customer.io, Resend), analytics platforms keep the tracker host at the workspace level and apart from
mail (Segment, Mixpanel, PostHog), and a few keep one link domain per account (Klaviyo, HubSpot).

## Decisions

- **A Tracking domain belongs to the Workspace.** It is a hostname the Owner chooses
  (`t.example.com`), set on the Tracking settings page, tied to no Sending domain. The host is any
  name below a registrable domain, never the apex (a CNAME cannot sit there) nor one of the Instance's
  own hosts (the app, the tracker, the CNAME target), stored lower case with IDN as punycode, and
  immutable: another host is another Tracking domain.
- **One host is used at a time.** While the Workspace's current Tracking domain is `serving`, the
  links of every tracked message (whatever its From domain or Integration) and the snippet are built on
  it; otherwise on the Instance's hosts. A Workspace is one brand here, so a host per Sending domain
  would buy a choice per send for a link host that takes no part in DMARC alignment; HubSpot sells a
  domain per brand as an add-on for the same reason.
- **A change takes over without a gap.** Setting another host while the current one serves makes it
  the replacement (one at a time; setting a third discards the pending one). The current host keeps
  serving new mail until the replacement reaches `serving`, then the replacement becomes current
  (Resend does the same).
- **A host that served is retired, not dropped.** A replaced or removed host that has served is
  retired: it serves no new mail or snippet, keeps serving the links already sent and the snippet
  already installed on it, and keeps its name from other Workspaces, until the recheck finds its
  CNAME no longer at the edge or the Owner forgets it. There is no timer: links do not expire
  (ADR 0036). A failed HTTPS probe does not end it, DNS does. A host that never served is deleted.
  Setting a retired host again brings it back and checks it anew. Platforms keep sent links working
  while the old CNAME stays (SendGrid, Klaviyo); breaking them on a change (Customer.io) is the
  exception.
- **Remove** retires (or deletes) the current host and promotes a pending replacement; **cancel**
  deletes the replacement; **forget** deletes a retired host. The Audit log is the trace (ADR 0049:
  configuration without a history trail).
- **The rest of ADR 0038 and 0059 stands, on the new entity:** the states, the CNAME to
  `SPHERICON_TRACKING_CNAME_TARGET` (empty turns the feature off), the probe, the recheck of every
  verified host, uniqueness across the Instance among verified hosts (retired included, `host_taken`),
  the gate that serves link paths, the tracker for the Workspace's own collect key and the probe, the
  `ask` endpoint, and the change log the edge reads, now keyed by host. A retired host is still served,
  so the log reports it `removed` only when it goes.
- **Owners are told** when the current host regresses, when a replacement takes over (keep the old
  CNAME while its links matter) and when a retired host goes because its CNAME did.
- **No license gate**, as before; a limit belongs to the platform that hosts the Instance.

## Considered options

A link domain per Sending domain plus a separate tracker host, the common split: two entities, two
CNAMEs and a choice per send. Several Tracking domains with a default, Iterable's model: the same
choice with a fallback. Remove-then-add for a change, Klaviyo's model: a window on the Instance's
host. Dropping a removed host at once: sent links break while the customer's CNAME still points at us.

## Consequences

The Sending domain loses its tracking record on every surface. The Tracking domain has its own
resource on `/site` and `/api`, the Permission `tracking_domains:manage` and the scopes
`tracking_domains:read` and `tracking_domains:write`. MCP gets every operation but remove, which on a
retired host breaks the links in mail already sent and so needs a person, as regenerating an
Integration's key does (ADR 0037). Hosts that were verified under the old model are
logged `removed` and not moved: there are no production Workspaces. The tracking guide in the
documentation repository is updated with the implementation.
