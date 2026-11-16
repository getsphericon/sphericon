---
status: accepted
---

# A client address is read only behind a trusted proxy

## Context

A request's client address keys the per-address rate limits (sign-up, sign-in, a failed bearer, the
collect API) and is kept as proof on consent records and Audit entries. It was the rightmost entry of
`X-Forwarded-For` whoever connected, on the assumption of one proxy in front of the binary. A client
that reaches the binary directly (the compose file publishes its port on every interface) names any
address it likes, and spends no limit of its own; two proxies in front put every client behind the
address of the inner one, in one bucket.

## Decisions

- **The proxies in front of the Instance are configured as ranges**: `SPHERICON_TRUSTED_PROXY_CIDRS`,
  by default loopback and the private networks, where a proxy on the same host, in a container
  network or in a cluster connects from, as web frameworks trust by default. A proxy on a public
  address is listed; an empty list trusts none.
- **`X-Forwarded-For` is read only when the connection comes from a trusted proxy**, and the client
  is its rightmost address outside the trusted ranges: every trusted hop is skipped, however many,
  and an entry a client wrote before them is never reached. A connection from anywhere else is its
  own client address.
- **The Tracking edge keeps its own list** (`SPHERICON_TRACKING_EDGE_CIDRS`, ADR 0038): the peers
  believed on the customer's host decide which Workspace a request serves, a narrower trust than an
  address.

## Consequences

- `clientip.Resolver` reads the address; the server builds it from the configuration.
- An Instance behind a proxy on a public address lists it, or every client is that proxy.

## Rejected

- **A count of trusted hops**: it is right only while the count matches the deployment, and a
  client that reaches the binary past the proxy is still believed.
- **One list for the proxies and the Tracking edge**: the private networks a proxy is trusted from
  by default would then be trusted to name the customer's host too.
