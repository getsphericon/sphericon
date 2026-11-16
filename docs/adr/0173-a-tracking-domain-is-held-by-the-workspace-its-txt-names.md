---
status: accepted
---

# A Tracking domain is held by the Workspace its TXT record names

> Amends [[0038-tracking-domains-are-core-and-the-edge-is-not]] (what verifies a host) and
> [[0071-a-tracking-domain-is-the-workspaces-own-not-a-sending-domains]] (how long a retired host
> stays).

## Context

A Tracking domain is a CNAME from a host of the Workspace's own to the Instance's edge (ADR 0038).
The check found the CNAME at the edge and made the host `dns_verified`, and a verified host serves
one Workspace across the Instance. The CNAME target is the same for every Workspace, so the CNAME
shows that a host points at the Instance, not which Workspace it is for: any Workspace that adds
the host and checks it while the CNAME stands passes the same check. Multi-tenant edges that serve
customers' hostnames keep a shared target and ask for a per-hostname ownership token beside it.

## Decisions

- **Each Tracking domain has an ownership token**: random, made with the row and never changed.
  The host publishes it in a TXT record `_sphericon-tracking.<host>` with the value
  `sphericon-tracking=<token>`, beside the CNAME. The API shows the record with the CNAME
  (`ownershipRecord`), as a Sending domain's DKIM record is shown.
- **A check needs both records, every time.** A current or replacement host is `pending` unless its
  CNAME reaches the edge and its TXT carries the row's token; with both it goes on to
  `dns_verified` and, once the probe answers, `serving`. A retired host stays while both stand and
  is deleted once either goes. So a host that changes hands leaves the Workspace that had it at its
  next check, and the Workspace whose token the host now publishes verifies it.
- **A TXT that names another Workspace's row is not a taken host.** Such a claim stays `pending`
  with no reason: the host is not this Workspace's, whoever holds it. `host_taken` remains for a host
  whose TXT names this row while another Workspace still holds it verified, until that Workspace's
  next check.
- **A resolver failure keeps the state**, on the TXT as on the CNAME: only a not-found answer is a
  missing record.

## Consequences

- `tracking_domains.ownership_token`, filled for the existing rows by its migration; the checker
  reads TXT records through the resolver Sending domains use; the settings show both records.
- An Instance's DNS and edge need nothing new: the CNAME target stays one for every Workspace.

## Rejected

- **A CNAME target per Workspace** (`<label>.<target>`): one record for the customer, but every
  Instance would need wildcard DNS for its target, and the check would have to read the host's first
  CNAME hop, since the resolver returns the end of the chain.
- **The TXT as a gate to leave `pending` only**: a host sold or handed to someone else would stay
  with the Workspace that verified it first, and its new owner would meet `host_taken` for good.
