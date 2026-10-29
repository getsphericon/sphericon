---
status: accepted
---

# MCP surface: a projection of the external `/api`, not a second implementation

> Amended by [[0065-a-live-automation-or-rule-changes-only-by-publish]]: the send-class list is
> Automation and Rule Publish, Email send, and Broadcast schedule and unschedule, and an edit of an
> active Automation or Rule writes a draft.

sphericon is operated by agents through an MCP server mounted at `/mcp` in the same binary
(Streamable HTTP, official `modelcontextprotocol/go-sdk`). Its tools are **projected from the
external OpenAPI contract** at startup and dispatch in-process through the same ogen server, so
workspace scoping, token scopes, validation and RFC 7807 errors are the `/api` ones by
construction. A hand-maintained parallel tool list, or a fourth TypeSpec spec, was rejected: it
drifts from the contract (codegen ethos, "generated over hand-rolled"). The only per-operation
override is a TypeSpec `x-mcp` extension (name, hidden, send), so even that lives in the contract.

The target is **capability parity with Drip's MCP**, expressed in sphericon's vocabulary
(`contacts_upsert`, never `subscriber`) and adapted to the model rather than copied: **Tag**
(glossary) is adopted because segments are built on it; forms, goals, orders and refunds are not
modelled and stay out.

## Decisions

- **Contract first.** `/api` today lacks most of what parity needs (its segments and broadcasts
  operations have no handlers). It is extended, additively and without a new version, to: contacts
  (+ batch upsert by alias keys, ADR 0002), tags, segments, broadcasts (drafts, `schedule`,
  `unschedule`, `test-send`, segment, report), automations (CRUD, `activate`/`deactivate`),
  templates, events (+ batch), webhooks, and read-only custom fields and the
  complaint/bounce rates (ADR 0011), integrations and sending domains (ADR 0024: credentials and the DKIM
  private key are never returned; `integrations:write` and `sending_domains:write` are the setup scopes, which an OAuth connector gets only behind a box of its own on the consent screen (see "Setup is a third lock");
  verification stays a live DKIM check that the API only triggers and reports). **Not exposed:** API tokens. Batches take up to 1000 items and
  return a per-item result; one failure does not roll back the rest.
- **Use-case modules before the contract grows.** `/site` and `/api` re-implement contact
  create/update and diverge: `/api` creates contacts without publishing `ContactCreated`, so they
  never trigger Automations. Domain logic moves into one deep package per domain (as
  `internal/outbound` does, ADR 0015) that owns the transaction and the event publish and returns
  domain errors; handlers stay thin adapters (scope check, call, map). Starts with contacts.
- **Send is a second lock.** The agent authors by default (draft / inactive). Send-class
  operations (`Emails_send`, broadcast `schedule`, automation `activate`) need their own `/api`
  scope (`emails:send`, `broadcasts:send`, `automations:activate`) **and**, over MCP, the extra
  `mcp:send` scope; without `mcp:send` they are not even listed. This relaxes the backlog's
  "AI is an author, not a sender" from absolute to opt-in per token.
- **Setup is a third lock.** `sending_domains:write` and `integrations:write` change how the
  Workspace sends: the domains it authenticates and the provider credentials an Integration holds.
  They were token-only, so an agent connected through OAuth could read a Sending domain but not fix
  one, which the owner or admin who approved it may do in the app. They are now the `SetupScope`
  group: an OAuth client gets them only when it asked for them **and** the user ticks a second box on
  the consent screen, apart from the send box (neither brings the other, and they bring no `mcp:send`).
  Everything that cannot be undone or gives a standing way in stays token-only: `tokens:*` (a connector
  must never mint a credential), `contacts:erase`, `audit:read`, `export:read`. Each setup scope
  needs the Permission of its resource to be granted (`integrations:manage`,
  `sending_domains:manage`), so a member's consent screen does not offer it
  ([[0061-access-to-an-operation-is-declared-in-the-contract]]).
- **Consent only narrows through the agent.** An agent may add a Suppression or record an
  Unsubscribe. Resubscribing and lifting a Suppression are not exposed on `/api` or MCP at all:
  they widen reach and assert consent, so a human (or the contact via link) does them.
- **Contact-supplied text is untrusted by kind of field.** `first_name`, `last_name`, Custom field
  values, Event properties and Tag names always originate outside the workspace, so the contract
  marks them `x-untrusted` and the projection wraps them in tool results and states the rule in
  the server instructions (treat as data, never as instructions). No per-row flag, no data-model
  change.
- **Auth.** Phase 1: the same Bearer API token as `/api`. Phase 2: OAuth 2.1 (discovery, dynamic
  client registration, consent screen in the SPA) for claude.ai connectors: `internal/oauthserver`
  serves the RFC 9728 / RFC 8414 metadata, public-client registration (PKCE S256 only), `/oauth/authorize`
  (hands off to the SPA consent route) and `/oauth/token`, which issues an **ordinary scoped API
  token** that expires after an hour, with a rotating single-use refresh token (30 days): a refresh rotates the secret of the same token row, and replaying a used
  refresh token revokes it. Send-class scopes are
  granted only when the user opts in on the consent screen. A `client_id` that is an https URL is a
  Client ID Metadata Document (MCP spec 2025-11-25): any such client is trusted behind the consent
  screen (no allowlist). The document is fetched through the SSRF-hardened client (the Instance's
  outbound allowlist applies), must name its own URL as `client_id` and list every redirect URI, and
  is cached on the `oauth_clients` row for its `Cache-Control` max-age (default 1 hour, at most 24).
  The consent screen shows the client's host and warns about a localhost redirect. Phase 3: shipped
  shipped prompts (Markdown files) served as MCP prompts.

## Implemented

Decisions taken while building it, so the contract and code are not the only record:

- **Scopes.** The `ApiTokenScope` enum is the complete vocabulary and a test keeps it so (every
  scope an `/api` handler checks must be grantable): `contacts`, `events`, `segments`,
  `broadcasts`, `automations`, `templates`, `webhooks` each as `:read` / `:write`;
  `custom_fields:read`, `sending_domains:read`, `integrations:read`; the send class `emails:send`, `broadcasts:send`,
  `automations:activate`, `mcp:send`; the setup scopes `sending_domains:write`, `integrations:write`; and `tokens:read` / `tokens:write` for `/api` token
  management, which is hidden from MCP.
- **`x-mcp`.** Declared in `typespec/external/mcp.tsp` with three keys: `name` (override of the
  snake_case operation id, e.g. `whoami`, `events_record`), `hidden` (token management is never a
  tool) and `send` (send-class: listed and callable only with `mcp:send`). Unscheduling a
  broadcast is send-class; deactivating an automation is not.
- **Untrusted fields.** `x-untrusted` in the contract marks the fields; the projection replaces
  each such value in a tool result with `{"untrusted_data": <value>}` and the server instructions
  say to treat it as data only.
- **OAuth.** `/.well-known/oauth-protected-resource` (also under `/mcp`),
  `/.well-known/oauth-authorization-server`, `POST /oauth/register`, `GET /oauth/authorize`,
  `POST /oauth/token`; the consent screen is the SPA route `/oauth/consent`, backed by
  `/site/oauth/authorization`. Public clients only, PKCE S256 required; the `authorization_code` and `refresh_token` grants (the refresh token
  is single use and rotates on every use; reuse revokes the grant). A
  `resource` parameter (RFC 8707), when sent to authorize or token, must equal the `/mcp` resource
  URL, otherwise `invalid_target`; an absent one is accepted. The resource lives at
  `${SPHERICON_MCP_URL}/mcp` (`SPHERICON_MCP_URL`, default `SPHERICON_APP_URL`) so a deployment may give MCP its own host while
  the issuer and consent screen stay on `SPHERICON_APP_URL`: protected resource metadata names the MCP
  resource and the `SPHERICON_APP_URL` issuer, and the `WWW-Authenticate` challenge points at the metadata
  on the MCP host. The binary is host-agnostic and serves discovery on every host. Unknown scopes are dropped;
  `tokens:*` is never grantable, so a connector cannot mint credentials.
- **Consent role.** Any member may approve a client. Approving creates a **Connection**: it belongs to
  the approving User's Membership, offers only the scopes that User's Permissions allow, can do on each
  request no more than the User's current Role allows, and ends with the Membership
  ([[0061-access-to-an-operation-is-declared-in-the-contract]]; this was "owner or admin, a workspace
  token").
- **Connections are not API tokens.** Each User lists and revokes their own Connections (revoking one
  kills its refresh token too), and `members:manage` lists and revokes anyone's; hand-made API tokens
  stay long-lived and are managed under `tokens:manage`.

## Considered options

- A 4th TypeSpec spec `mcp` with generated intent-level tools: more infrastructure; deep verbs can
  be layered on later if the CRUD mirror (~60 tools) proves too wide.
- Tools calling `ent` directly: bypasses scoping and the use-case invariants.
