# Prometheus metrics are served on an opt-in internal listener, never on the public port

`/metrics` used to be mounted unauthenticated on the public mux, with protection delegated to the
ingress. It is now served only by a dedicated listener bound to `SPHERICON_METRICS_ADDR` (`host:port`,
empty by default = no listener, same shape as RudderStack's opt-in Prometheus port). There is no
`/metrics` route on the public mux, no token and no allowlist: the network boundary is the
control. A `SPHERICON_METRICS_ADDR` equal to the public `PORT` is a configuration error, and a bind failure
is fatal, because a self-hoster who asked for metrics must not silently run without them.
`/healthz` and `/readyz` stay unauthenticated on the public port (orchestrator probes cannot send
credentials).

## Considered Options

- **Bearer token on the public port (Gitea).** Rejected: a secret to rotate, and an empty token
  silently means public. The listener is safe by construction.
- **IP allowlist (GitLab).** Rejected: behind a proxy the client address is not the scraper's.
- **Keep it public and document ingress restriction.** Rejected: the default `docker run` exposes it.

## Consequences

Metric labels must be bounded technical dimensions (handler, route template, status class,
outcome). Tenant or personal identifiers (workspace id or slug, contact, email, recipient id) are
forbidden as labels: they leak across tenants and explode cardinality. Per-tenant analytics come
from Events in Postgres (ADR 0011), not from `/metrics`.
