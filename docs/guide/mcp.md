# MCP for agents

Sphericon serves a [Model Context Protocol](https://modelcontextprotocol.io) endpoint at `/mcp`, so an
AI agent (Claude, or any MCP client) can operate a workspace: read contacts, draft broadcasts,
build segments, author automations. It is the same binary and the same data as `/api`.

The tools are **projected from the external API contract**, not written by hand. Every operation
in the [API reference](/api/) is a tool, named after its operation id in snake_case
(`Contacts_list` becomes `contacts_list`). Token scopes, workspace scoping, validation and error
bodies are the `/api` ones.

## Connect

The endpoint speaks Streamable HTTP and is stateless. Create an API token under
**Settings → Developers** (see [scopes](/guide/api#tokens-and-scopes)), then point your client at
`https://sphericon.example.com/mcp` with the token as a Bearer credential.

If you serve MCP from its own host (`SPHERICON_MCP_URL`, for example `https://mcp.sphericon.example.com`),
use `${SPHERICON_MCP_URL}/mcp` instead; sign-in and consent still happen on the app host.

Claude Code:

```sh
claude mcp add --transport http sphericon https://sphericon.example.com/mcp \
  --header "Authorization: Bearer $SPHERICON_TOKEN"
```

Any client that takes a JSON configuration:

```json
{
  "mcpServers": {
    "sphericon": {
      "type": "http",
      "url": "https://sphericon.example.com/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

### Connectors that sign in (OAuth)

Clients that cannot hold a static header, such as claude.ai custom connectors, use OAuth 2.1:
add `https://sphericon.example.com/mcp` as the connector URL and approve the consent screen in the
Sphericon app. Sphericon serves the discovery metadata (RFC 9728 and RFC 8414), dynamic client
registration and PKCE (S256 only).

- Only a workspace **owner or admin** can approve, because approval mints a token.
- The result is an ordinary API token named `<client> (MCP)`. Its access token lasts one hour and
  the connector renews it with a rotating refresh token (30 days, single use; replaying a used one
  revokes the connector). The row stays the same in the workspace settings; revoke it there like
  any other token and both tokens stop working.
- The consent screen lets you opt in to the send-class scopes. Token management scopes
  (`tokens:*`) can never be granted to a connector.

## What an agent can do

| Area                | Tools (prefix)                                                                         |
| ------------------- | -------------------------------------------------------------------------------------- |
| Identity            | `whoami`                                                                               |
| Contacts            | `contacts_*`, `contacts_upsert_batch`, `tags_*`                                        |
| Events              | `events_record`, `events_record_batch`, `events_actions_list`                          |
| Segments            | `segments_*`, `segments_preview`                                                       |
| Templates           | `templates_*`                                                                          |
| Writing mail        | `brand_get`, `email_schema`, `email_lint`                                              |
| Broadcasts          | `broadcasts_*`, `broadcasts_set_audience`, `broadcasts_test_send`, `broadcasts_report` |
| Automations         | `automations_*`, `automations_deactivate`, `automations_dry_run_http_step`             |
| Enrollments         | `enrollments_list`, `enrollments_get`                                                  |
| Reference and rates | `custom_fields_list`, `sending_domains_list`, `sending_domains_rates`                  |
| Consent             | `unsubscribes_create`, `suppressions_create`                                           |
| Webhooks            | `webhooks_*`, `webhooks_regenerate_secret`, `webhooks_test`                            |

API token management is never a tool. The live list for your token is whatever `tools/list`
returns; it depends on the token's scopes.

## Safety model

- **Authoring, not sending, by default.** Broadcasts and automations are created as drafts.
  The send-class tools (`emails_send`, `broadcasts_schedule`, `broadcasts_unschedule`,
  `automations_activate`) need their own API scope (`emails:send`, `broadcasts:send`,
  `automations:activate`) **and** the extra `mcp:send` scope on the token. Without `mcp:send`
  they are not even listed.
- **Consent only narrows.** An agent can suppress an address or record an unsubscribe. Resubscribing
  and lifting a suppression are not available over `/api` or MCP at all.
- **Contact text is data.** Contact names, custom field keys and values, event properties, tag
  names and the Brand kit's fonts, logo address and footer text come from outside the agent's
  instructions. In tool results they are wrapped as
  `{"untrusted_data": …}`, and the server instructions tell the model to treat them as data and
  never as instructions.

## Writing mail

An agent writes mail in the editor's format, the same document the editor in the app saves.
`brand_get` returns the Workspace's Brand kit (colors, fonts, button style), `email_schema` the
node and mark types of the document with the attributes and values allowed, and `email_lint`
checks a message (a `body` or a `doc`) and returns errors and warnings without saving anything.
An agent copies a starter Template (`templates_get` returns its `doc`), edits the document and
writes it as `doc` to `templates_create`, `broadcasts_create` or an email step of
`automations_create`. The server renders the body from the document in the same save and refuses
an invalid one with the reason (`422`); nothing is repaired or stored. A message sent only as a
`body` is stored as HTML with no document. Writing a `body` and a `doc` together is a `422`.

## Playbooks

The server also publishes MCP prompts that walk an agent through common jobs, always leaving the
final send to a human:

- **welcome**: draft a welcome automation for new contacts.
- **win-back**: draft a broadcast for contacts who went quiet.
- **list-hygiene**: audit audience health and propose cleanup.
