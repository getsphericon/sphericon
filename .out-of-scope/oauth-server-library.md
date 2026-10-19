# Replacing the hand-written OAuth server with a library

This project keeps its own OAuth 2.1 authorization server for MCP connectors rather than adopting
`ory/fosite` or `zitadel/oidc`.

## Why this is out of scope

What an MCP authorization server must do (MCP spec 2025-11-25) is mostly the part no library covers:
dynamic client registration, authorization server metadata, `resource` (RFC 8707) validation and
Client ID Metadata Documents. Checked in the sources: fosite has none of them; zitadel/oidc only
declares the `registration_endpoint` and `client_id_metadata_document_supported` fields in its
discovery document and ships no handler behind them. What a library would replace is the token
endpoint mechanics, about 60 lines that already use `oauth2.S256ChallengeFromVerifier` and a
constant-time compare, with single-use codes claimed by one conditional UPDATE. Adopting either
library would add adapters from their storage interfaces to ent and still leave the MCP-specific
endpoints hand-written. Ory Hydra, the service built on fosite, breaks the single-binary rule.

Revisit if a library gains DCR, metadata and resource indicators, or if the grant logic grows
beyond the authorization-code and refresh-token grants.

## Re-check after refresh tokens and rotation (#361)

Refresh tokens with rotation and reuse detection are now in (`internal/oauthserver/grants.go`).
They reuse the claim-one-row shape of the code grant, on the connector's
`ApiToken` row, so the grant logic did not outgrow the hand-written server. Re-checked once:
`ory/fosite` has had no release since v0.49.0 (2024-12) and still has no DCR, metadata documents or
resource indicators; `zitadel/oidc` (v3.51.16) still only declares the discovery fields. Still no fit.

## Prior requests

- #340: "Evaluate replacing the hand-rolled OAuth server with a library"
- #361: "Short-lived MCP access tokens with refresh"
