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

Revisit if a library gains DCR, metadata and resource indicators, or if refresh tokens and rotation
are added and the grant logic grows beyond the authorization-code flow.

## Prior requests

- #340: "Evaluate replacing the hand-rolled OAuth server with a library"
