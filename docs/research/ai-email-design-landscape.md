# AI Email Design Landscape (how competitors get beautiful emails out of agents)

Researched 2026-10-18 from vendor documentation, vendor blog/changelog posts and source
repositories (all fetched on that date). Third-party roundups were used only as leads and are
listed in "Unverified". Tool names are quoted exactly as the vendor's docs or source list them.
Where a vendor page was a summary rather than a tool list, that is said. The sphericon-side
claims are grounded in files read in this repo (listed in section 5); where they disagree with
`GLOSSARY.md` or `docs/adr/*`, the ADRs win.

Scope: this note maps what exists and the trade-offs. It makes no scope recommendation: the
owner decides in a grill.

## 1. Summary

- Every vendor that ships good-looking agent output **narrows the vocabulary the agent writes**
  rather than letting it emit free HTML: Beefree (block tools), Klaviyo (drag-and-drop template
  definition), Loops (LMX tags with theme/component ids), Resend (editor JSON with a schema the
  agent must fetch), Unlayer (Elements component schema), Stripo (saved template + modules),
  Keila (template content slots). Raw HTML is the fallback, and Resend and Klaviyo both say raw
  HTML is the worse path for editing.
- **Brand is data the platform holds and enforces**, not text in a prompt: Beefree `brandRules`
  (`presets`, `permissions`, `limits`, presets are mandatory once defined), Loops themes and
  components, Klaviyo brand tools (beta), Mailchimp brand kit, Stripo business profiles.
- **The agent's feedback loop is thin almost everywhere.** Linting is common (Beefree
  `beefree_check_template`, Unlayer `diagnose_template`, Postmark `validateTemplate`, Customer.io
  dry-run, Loops strict LMX validation); **image preview returned to the agent is rare** (Unlayer
  `render_preview_image` and `previewImageUrl`; Klaviyo only has `render_email_template`, whose
  output type the docs do not state). Stripo's own blog shows an agent that "could not retrieve
  those previews".
- **Review and send gating differ sharply.** Customer.io is the closest to sphericon's design:
  default `read`-only scope, a separate `write` (drafts) scope and a separate `write:live`
  (send) scope that an admin must first enable. Klaviyo has an opt-in `read-only=true` mode and a
  `send_campaign` tool. Postmark and Resend document no sub-scoping. Stripo and Beefree expose
  no send tool at all in what was read.
- **Open-source tools mostly have no first-party AI surface** (Listmonk, Keila, Mautic,
  Dittofeed: nothing official found; Novu has an MCP server and skills, Cloud only). Community
  MCP servers exist for Listmonk and Mautic and are the only place preview-first send gating
  shows up in that group. Keila's June 2026 content slots are a notable template-locking
  primitive that fits MJML.
- Sphericon today gives an agent MJML plus a template list: no brand data, no vocabulary, no
  lint, no preview back, and an MJML error surfaces only when a broadcast is planned to send.

## 2. Comparison table

"Constrains the agent" = what limits what it can write. "Sees the result" = what comes back to
the agent. "Send authority" = what the docs say about who may send. Cells say "not documented"
when the pages read do not say; that is not a claim of absence.

| Vendor      | Constrains the agent                                                                       | Agent sees the result via                                                          | Human review step                          | Send authority via agent                           |
| ----------- | ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- | ------------------------------------------ | -------------------------------------------------- |
| Resend      | TipTap JSON schema fetched with `get-tiptap-json-content`; raw HTML is a lossy alternative | Editor presence and shared live document; no preview tool listed                   | Dashboard editor (agent edits appear live) | `send-broadcast` exists; no gating in docs read    |
| Loops       | LMX tags, theme ids, component ids, strict validation                                      | API validation errors; preview email to an address                                 | Draft campaigns; editor                    | MCP: not stated; API can schedule a draft campaign |
| Beefree     | Block tools plus `brandRules` presets/permissions/limits                                   | `beefree_check_template`, `beefree_check_section`, `beefree_get_content_hierarchy` | Live editor session with the human         | No send tool in the documented list                |
| Unlayer     | Elements schema (`get_elements_schema`)                                                    | `render_email`, `render_preview_image`, `diagnose_template`                        | Every design opens in the visual builder   | No send tool in the 14 listed                      |
| Stripo      | Saved template, modules, brand rules supplied via skills/files                             | Export HTML and screenshots (sometimes unavailable)                                | Edit in Stripo, team approval              | MCP does not schedule or send                      |
| Klaviyo     | DND template definition, brand tools (beta)                                                | `render_email_template`, `create_template_preview_send_job` (beta)                 | Composer proposals reviewed in preview     | `send_campaign` tool; `read-only=true` opt-in      |
| Mailchimp   | Brand kit feeds AI flow templates                                                          | Flow editor                                                                        | Review, edit or turn the flow on           | Official MCP is docs-search only                   |
| Customer.io | Skills library; strict body validation                                                     | `dry-run` on write tools                                                           | Drafts in Design Studio                    | `write:live` scope, admin opt-in                   |
| Braze       | Template tools; BrazeAI Operator in dashboard                                              | `get_campaign_draft` (beta)                                                        | Dashboard permissions                      | `launch_campaign` (beta program)                   |
| Brevo       | Not documented                                                                             | Not documented                                                                     | Not documented                             | Not documented                                     |
| Postmark    | Layout plus standard template binding                                                      | `validateTemplate`                                                                 | Not documented                             | Server token = full permissions                    |
| Listmonk    | None official                                                                              | `GET /api/templates/{id}/preview`                                                  | Admin UI                                   | Community servers only                             |
| Keila       | Content slots in templates                                                                 | Preview, 5-recipient test                                                          | Campaign editor                            | No AI surface found                                |
| Mautic      | Theme owns skeleton (community plugin)                                                     | Community: preview URLs, dry run                                                   | Builder                                    | Community: `confirmSend`                           |
| Dittofeed   | None                                                                                       | None                                                                               | Editor                                     | LLM integration listed unchecked                   |
| Novu        | React Email in workflows                                                                   | Not documented                                                                     | Not documented                             | Cloud-only MCP                                     |
| MJML        | Validation levels `strict`, `soft`, `skip`                                                 | Compiler errors                                                                    | n/a                                        | n/a                                                |

## 3. Per-competitor notes

### Resend (React Email 6, editor, MCP, skills)

- **React Email 6.0** (blog dated 2026-04-17, updated 2026-07-09): a new open-source visual
  editor as the standalone package `@react-email/editor`, whose "editor outputs semantically
  correct, email-ready HTML that renders correctly across every major inbox provider"; a
  composable `EmailNode` extension API for custom blocks; new templates; the post does not
  mention AI, agents, MCP or skills.
  <https://resend.com/blog/react-email-6.md> (accessed 2026-10-18)
- **AI editor** (2026-04-14): "Give the editor any URL and the AI will extract your brand, voice,
  and tone"; before send the editor "reviews your content" for deliverability, content
  compatibility, subject and preview, broken links and typos; with the MCP server "you can give
  your own agent direct access to the visual editor".
  <https://resend.com/blog/ai-email-editor.md> (accessed 2026-10-18)
- **New editor** (2026-03-12): global email theming ("define global styles for your emails in one
  place"), multi-column layouts, an HTML code view.
  <https://resend.com/changelog/introducing-the-new-email-editor.md> (accessed 2026-10-18)
- **MCP server**: hosted `https://mcp.resend.com/mcp` (OAuth, or API key as Bearer) or local
  `npx -y resend-mcp`. The docs page groups tools by resource and does not give names; send
  gating is not described there.
  <https://resend.com/docs/mcp-server.md> (accessed 2026-10-18)
- **Tool names, from the source repo** (`src/tools/*.ts`, registered with `registerTool`):
  editor: `get-tiptap-json-content`, `connect-to-editor`, `disconnect-from-editor`; templates:
  `create-template`, `list-templates`, `get-template`, `compose-template`, `update-template`,
  `remove-template`, `publish-template`, `duplicate-template`; broadcasts: `create-broadcast`,
  `send-broadcast`, `list-broadcasts`, `get-broadcast`, `cancel-broadcast`,
  `duplicate-broadcast`, `remove-broadcast`, `compose-broadcast`, `update-broadcast`,
  `list-broadcast-clicked-links`, `list-broadcast-recipients`.
  <https://github.com/resend/resend-mcp> and the raw files under
  <https://raw.githubusercontent.com/resend/resend-mcp/main/src/tools/> (accessed 2026-10-18)
- **How the agent is constrained** (tool descriptions in `broadcasts.ts`): the documented
  workflow is `list-segments -> create-broadcast -> get-tiptap-json-content (include_schema: true)
  -> compose-broadcast -> send-broadcast`. `get-tiptap-json-content` returns the current document
  plus "the TipTap schema reference" and is described as "Required for producing valid TipTap
  JSON"; `compose-broadcast` is "the recommended way to set email content"; setting raw
  `html`/`text` through `update-broadcast` is "lossy" and the agent is told to ask the user
  first. `create-broadcast` "does NOT send yet".
- **Skills**: three Agent Skills (Email Best Practices, React Email, Resend), installed with
  `npx skills add resend/email-best-practices`, `npx skills add resend/react-email`,
  `npx skills add resend/resend-skills`; the React Email skill is described as giving
  "Tailwind CSS integration for brand-aligned designs" and a "development preview server with hot
  reloading".
  <https://resend.com/blog/introducing-email-skills.md> (accessed 2026-10-18)
- **Send gating**: nothing found beyond `create-broadcast` not sending and a sender-address
  prompt. The README's only safety notes concern local file attachment paths
  (`--allowed-file-dirs`).

### Loops (LMX, MCP, skills)

- **LMX** is "an XML-based format for writing email content in Loops", every element an explicit
  PascalCase tag, usable through the Content API, the CLI and the editor. Validation is strict:
  "Unknown tags are rejected"; child-only tags are rejected at the wrong level; a variable such
  as `{contact.firstName}` needs a prefix; an `Image` `src` must be a Loops-hosted URL.
  Styling is by reference: `<Style themeId="st_default" .../>` and `<Component componentId="logo" />`,
  with `list themes` and `list components` endpoints so the agent can discover the ids.
  <https://loops.so/docs/creating-emails/lmx.md> (accessed 2026-10-18)
- **Optimistic concurrency**: update email message needs `expectedRevisionId` equal to the
  current `contentRevisionId`, else 409.
  <https://loops.so/docs/api-reference/update-email-message.md> (accessed 2026-10-18)
- **Campaigns**: "Create a draft marketing campaign and its associated email message"; "To send
  a test preview of a campaign, use the Preview email message endpoint" (limit 100 preview emails
  per day per team).
  <https://loops.so/docs/api-reference/create-campaign.md>,
  <https://loops.so/docs/api-reference/preview-email-message.md> (accessed 2026-10-18)
- **API scheduling**: update-campaign accepts `scheduling` with `method: schedule` and a future
  `timestamp`, so the API can schedule a draft campaign.
  <https://loops.so/docs/api-reference/update-campaign.md> (accessed 2026-10-18)
- **MCP server** (`https://mcp.loops.so`, OAuth with Client ID Metadata Documents): lists
  contacts, mailing lists, events, transactional email ("send a published transactional email"),
  and "create and manage campaigns and workflows, and edit email content". The page gives no
  tool names, no statement on whether the MCP can send or schedule a campaign, and no read-only
  mode. <https://loops.so/docs/mcp-server.md> (accessed 2026-10-18)
- **Skills**: four, including an "LMX skill" ("teaches your agent Loops' markup language") and an
  "Email best practices skill" (deliverability, lifecycle coverage, consent handling).
  <https://loops.so/docs/skills> (accessed 2026-10-18)

### Beefree (MCP server)

- **Tool surface** (`beefree_*`): `beefree_add_section`, `beefree_delete_element`,
  `beefree_update_section_style`, `beefree_update_column_style`, `beefree_get_content_hierarchy`,
  `beefree_get_element_details`, `beefree_set_email_metadata`,
  `beefree_set_email_default_styles`; content: `beefree_add_title`, `beefree_add_paragraph`,
  `beefree_add_list`, `beefree_add_image`, `beefree_add_icon`, `beefree_add_button`,
  `beefree_add_social`, `beefree_add_menu`, `beefree_add_spacer`, `beefree_add_divider`,
  `beefree_add_table` (each with update tools); QA: `beefree_check_template`,
  `beefree_check_section`; `beefree_search_stock_images`. The checker is described as covering
  "accessibility issues, missing alt text, color contrast problems, broken links".
  <https://docs.beefree.io/beefree-sdk/ai-and-mcp/getting-started/mcp-server-tools-and-capabilities.md>
  (accessed 2026-10-18)
- **Brand Rules for AI**: a `brandRules` object on an AI editing session with `presets`
  (named styles, "Presets are mandatory once defined", "Preset values take precedence"),
  `permissions` (`noAdd`, `noUpdate`, `noDelete` per block type) and `limits` (`rows.max/min`,
  `columns.maxPerRow`, `blocks.max`, `blocks.maxPerType`). Available on Superpowers and
  Enterprise plans. Each preset carries a written `description` the agent reads to choose.
  <https://docs.beefree.io/beefree-sdk/ai-and-mcp/brand-rules-for-ai.md> (accessed 2026-10-18)
- **Recommended agent workflow** (their system-prompt guide): `beefree_get_content_hierarchy`,
  `beefree_set_email_default_styles`, add sections and blocks, then `beefree_check_section` after
  each major section; prompt principles include "one primary CTA", "14px+ body text, 44px+
  buttons", "descriptive alt text, strong contrast".
  <https://docs.beefree.io/beefree-sdk/ai-and-mcp/getting-started/how-to-get-the-best-ai-output.md>
  (accessed 2026-10-18)
- **Quality Check** (the human-facing counterpart): default limits "Maximum image weight: 500 KB
  for email" and "Maximum HTML size: 80 KB for email HTML", with a comment that large HTML "can
  cause messages to get clipped in Gmail".
  <https://docs.beefree.io/beefree-sdk/qa-tools/quality-check.md> (accessed 2026-10-18)
- **Access**: the FAQ says the MCP server is available on Essentials, Core, Superpowers and
  Enterprise plans and that "Access is available directly to all paying customers with no
  application or approval needed"; calls are billed under CSAPI entitlements; "Not all content
  blocks are supported yet". A search snippet claimed invite-only beta access; the
  `early-access` page it cited returned 404 on 2026-10-18, so the FAQ is taken as current.
  <https://docs.beefree.io/beefree-sdk/ai-and-mcp/getting-started/mcp-server-faqs.md>
  (accessed 2026-10-18)
- **Shape**: the MCP works against a live editor session the host application opens
  (`bee.startMcpSession()` or an API-managed session); the host supplies the agent. A "Code Mode"
  (open beta) replaces 33 tools with one TypeScript-script tool, reported to cut token use 68-96%
  (vendor benchmark). No send tool appears in the documented list.

### Unlayer

- **MCP** `https://api.unlayer.com/mcp`, OAuth 2.1 with PKCE, "14 Unlayer tools". Names:
  `diagnose_template`, `get_template`, `list_templates`, `ai_generate_design`,
  `import_html_to_design`, `export_design`, `render_email`, `render_preview_image`,
  `get_elements_schema`, `render_elements`, `get_my_subscription`, `list_recent_releases`,
  `list_workspaces`, `search_documentation`. OAuth scopes: `templates:read`, `designs:write`,
  `render:execute`. `diagnose_template` checks "missing alt text, low contrast, oversized images,
  and malformed merge tags" and the page says it "is not a full accessibility audit". Designs
  from `ai_generate_design` open in "a temporary editor session"; `previewImageUrl` is returned
  when rendering succeeds, which is the one documented case of the agent getting an image.
  "Every design opens in the Unlayer visual builder, so teammates can edit what an agent created."
  <https://docs.unlayer.com/ai/mcp> (accessed 2026-10-18). A search snippet said this server was
  unreleased; the page now describes it as live.
- No send or schedule tool is among the 14.

### Stripo

- **MCP** `https://mcp.stripo.email/mcp` (plan Medium, Pro or Prime; Owner or Admin enables it):
  the assistant can "find and read emails, templates and modules; create and edit emails;
  generate and edit images; export HTML and screenshots; and delete content", with "your own
  Stripo permissions". A free **Stripo Agent Plugin** adds skills "for creating emails from a
  brief or reference, editing existing content, and maintaining business profiles".
  <https://support.stripo.email/en/articles/15921958-how-to-connect-ai-tools-to-stripo-via-mcp-integration>
  (accessed 2026-10-18)
- **Send**: "Stripo MCP does not schedule or send campaigns. After creating or editing an email,
  follow your team's approval and sending process."
  <https://stripo.email/blog/mcp-for-email-marketing-what-you-can-do-with-stripo/> (accessed
  2026-10-18)
- **Constraint pattern shown by Stripo's own examples**: start from a saved template that already
  holds the brand header and footer, pass brand rules as a file, tell the agent "Do not add extra
  sections"; "review the saved email visually before sending". In one example the assistant
  "could not retrieve those previews, so the visual review still had to be done in the editor."
  (same blog post)
- REST API: "Generate an email" builds an email from a saved template and saved modules filled
  with external data, so the design vocabulary is the project's own library.
  <https://api.stripo.email/reference/generateemail> (read as a search result only; not fetched
  in full, see Unverified).

### Klaviyo (Composer, MCP)

- **MCP** `https://mcp.klaviyo.com/mcp`: "over 260 tools". Relevant ones as the docs list them:
  campaigns `create_campaign`, `assign_template_to_campaign_message`, `update_campaign_message`,
  `send_campaign` ("Send a campaign to its configured audience"), `cancel_campaign_send`,
  `get_campaign_recipient_estimation`; templates `create_email_template` (HTML),
  `create_dnd_email_template` ("Create a drag-and-drop (DND) email template with a structured
  definition"), `update_dnd_email_template`, `render_email_template` ("Render an email template
  with provided context"), `create_template_preview_send_job` (Beta); brands (Beta)
  `create_brand_button`, `create_brand_color`, `create_brand_logo`, `create_brand_social_group`.
  <https://developers.klaviyo.com/en/docs/klaviyo_mcp_server_available_tools> (accessed 2026-10-18)
- **Gating**: `read-only=true` (remote) or `READ_ONLY=true` (local) disables write tools; the
  remote server loads a core tool set by default (`core-tools-only`); `toolsets=` limits by API
  scope; `disable-tools-with-user-generated-content=true` hides tools that read profile or event
  text. Read-only is opt-in, not the default.
  <https://developers.klaviyo.com/en/docs/connect_to_the_klaviyo_mcp_server> (accessed 2026-10-18)
- **DND templates**: Klaviyo's blog states the point directly: AI "outputs Klaviyo-native
  drag-and-drop templates instead of raw HTML" because "400 lines of raw HTML" are not something a
  marketer can edit; create/update tools are for Enterprise, Commercial and internal users.
  <https://www.klaviyo.com/blog/create-ai-designed-email-templates-you-can-actually-edit>
  (accessed 2026-10-18)
- **Composer** (in-app AI agent): drafts "grounded in your actual historical performance data and
  brand context"; "Composer does not send or publish campaigns, flows, or messages without your
  sign-off"; "Composer hands drafts back to you in the editor, fully editable"; a pre-send
  quality check across links, audience, send settings and compliance; "best for content built in
  Klaviyo's drag-and-drop editor" and weaker on image-heavy templates.
  <https://help.klaviyo.com/hc/en-us/articles/52230280693403> (accessed 2026-10-18)
- Finding on the earlier conflict: the MCP **does** expose `send_campaign`; the in-app Composer
  does not send.

### Mailchimp (Intuit Assist)

- Intuit AI "will access your brand kit to generate content and add styles to your flow
  template"; the user can "review the customized content in the automation flow, choose to make
  edits, or turn your flow on right away"; beta, Standard plan or higher, US primary mailing
  address required.
  <https://mailchimp.com/help/create-pre-built-journey-emails-intuit-assist> (accessed 2026-10-18)
- The official Marketing-API MCP server is documentation search only: "This searches
  documentation, not your Mailchimp account."
  <https://mailchimp.com/developer/marketing/build/ai-tools/mcp-server> (accessed 2026-10-18)
- A separate Mailchimp Transactional MCP exists (`https://mandrillapp.com/mcp`) per a search
  snippet; not fetched.

### Customer.io (MCP)

- A hosted MCP server exists (the earlier doubt is resolved). Tools: `cio_prime`, `cio_schema`,
  `cio_read_api`, `cio_write_api`, `cio_delete_api`, `cio_skills_list`, `cio_skills_read`,
  `cio_auth_status`. Read, write and delete are separate tools "so you can approve each category
  independently"; write and delete support dry-run previews; "Requests with unknown fields in the
  body return a 422".
  <https://docs.customer.io/ai/mcp/get-started/> (accessed 2026-10-18)
- **Scopes**: "By default, a connection only gets the read scope"; `write` = "Create, edit, and
  delete drafts: automations, segments, one-time sends, templates, people"; `write:live` = "Send
  messages, manage subscriptions and suppressions", "Doesn't include editing content", "Only
  available if your account admin has turned on Allow MCP to edit live data"; an agent that edits
  content and sends "must request both". `configure` is a fourth, independent scope. The MCP is
  enabled per account by an admin and the user cannot grant more than their own permissions.
  <https://docs.customer.io/ai/mcp-server> (accessed 2026-10-18)
- Skills library fetched through `cio_skills_read` carries multi-step workflows such as "Draft a
  welcome email in Design Studio". Page labelled beta.

### Braze (MCP, BrazeAI)

- Remote MCP server with OAuth tied to the dashboard user; "These tools do not return data from
  Braze user profiles"; the local server is deprecated.
  <https://braze.com/docs/developer_guide/mcp_server/> (accessed 2026-10-18)
- Tools include `get_email_templates`, `get_email_template_info` ("Drag-and-drop editor templates
  are not accepted"), `create_email_template`, `update_email_template`; campaign tools
  `create_campaign`, `edit_campaign`, `launch_campaign`, `stop_campaign`, `get_campaign_draft`,
  `get_campaign_live_details` are flagged "only available to customers participating in the
  campaign APIs beta program"; Canvas tools are read-only.
  <https://braze.com/docs/developer_guide/mcp_server/available_api_functions/> (accessed 2026-10-18)
- Email generation: BrazeAI "generative AI capabilities (copywriting, images, Liquid, HTML
  templates, and content review) are reached through Operator", inside the dashboard.
  <https://braze.com/docs/user_guide/brazeai/> (accessed 2026-10-18). Earlier secondary claims
  about Braze email generation are therefore only partly confirmed: it is in-dashboard Operator,
  not an MCP generation tool.

### Brevo

- MCP at `https://mcp.brevo.com/v1/brevo/mcp` ("All features combined (27 modules)") plus
  individual servers (contacts, `email_campaign_management`, `campaign_analytics`, `templates`,
  ...); authenticated by an MCP-flagged API key as Bearer. The page names no tools, no design
  vocabulary and no send gating.
  <https://developers.brevo.com/docs/mcp-protocol> (accessed 2026-10-18)

### Postmark

- Official MCP server by ActiveCampaign (`github.com/ActiveCampaign/postmark-mcp`): 24 tools;
  templates CRUD plus `validateTemplate` ("Validates template content (Mustachio syntax,
  undefined variables) without saving"); Layout templates bind Standard templates. MCP
  annotations (`readOnlyHint`, `destructiveHint`) let clients auto-approve reads. The README
  warns it "acts with the full permissions of the configured POSTMARK_SERVER_TOKEN" and that
  neither token type supports sub-scoped permissions.
  <https://github.com/ActiveCampaign/postmark-mcp> (README read 2026-10-18)

### Open source

- **Listmonk**: official docs show template `type` values `campaign`, `campaign_visual`, `tx` and
  `GET /api/templates/{id}/preview`; no official MCP or AI feature found.
  <https://listmonk.app/docs/apis/templates/> (accessed 2026-10-18). Community servers exist
  (for example `antoniolg/listmonk-mcp`, `mnbro/listmonk-mcp-bridge`); their README claims are
  not vendor claims.
- **Keila**: no AI or MCP found. Relevant primitive: since v0.30.0 (2026-06-24) templates in MJML,
  HTML or plain text with `<keila-content name="main">` **content slots**; the campaign editor
  "lets you only edit the slot content, allowing you to maintain consistent header/footer
  design"; in MJML templates slots are direct children of `<mj-body>`; the API accepts
  `mjml_content`. Since v0.17.0 (2025-03-14) preview toggles mobile and desktop and can email a
  preview to five recipients.
  <https://www.keila.io/updates/content-slots/>, <https://www.keila.io/updates/improved-previews/>
  (accessed 2026-10-18)
- **Mautic**: no official AI or MCP surface found. Community: `Bloomidea/mautic-plugin-ai-email-sections`
  generates MJML sections inside the builder ("The theme still owns the brand and the skeleton;
  this fills one section at a time"); several community MCP servers, one with `preview_email_send`
  / `dryRun` then `confirmSend: true` (as described by a search result; not read in the repo).
  <https://github.com/Bloomidea/mautic-plugin-ai-email-sections> (README read 2026-10-18)
- **Dittofeed**: editor supports Liquid, HTML and MJML; the roadmap lists "LLM integration"
  (Q3 2025) with an unchecked box. No MCP found.
  <https://github.com/dittofeed/dittofeed> (README read 2026-10-18)
- **Novu**: an account MCP server and agent skills exist (Cloud only: "Novu does not currently
  publish a self-hosted account MCP server"); email content is React Email in workflow code. No
  design-specific tooling read.
  <https://docs.novu.co/platform/build-with-ai/mcp> (accessed 2026-10-18)
- **MJML itself**: the compiler offers `validationLevel` of `strict`, `soft` or `skip`
  (default `soft`). <https://documentation.mjml.io/> (accessed 2026-10-18). No official MJML skill
  or MCP found; community projects exist (`shaunie2fly/mjml_mcp`,
  `framix-team/skill-email-html-mjml`), seen only as search results.

## 4. Patterns that make AI emails look good

Each pattern is cited to the vendors above; "inference" marks a conclusion drawn here.

1. **Constrained vocabulary, not free markup.** Beefree block tools with per-block add/read/update/
   delete; Loops LMX rejects unknown tags; Resend hands the agent a schema first; Unlayer
   `get_elements_schema`; Klaviyo structured DND definition. Klaviyo and Resend both justify it by
   human editability, not only quality.
2. **Brand as enforced data.** Beefree presets override the agent's own choices ("the AI is told
   which values were overridden"); Loops themes/components are referenced by id; Stripo keeps
   header and footer in a saved template; Keila slots lock everything outside the editable region.
   Inference: the common move is that brand never passes through the model's output channel.
3. **Template/slot locking.** Keila content slots, Stripo reference template ("Keep its header and
   footer"), Mautic plugin ("fills one section at a time"), Postmark layouts. The agent writes
   only the variable region.
4. **Render feedback loop.** Only Unlayer documents an image back to the agent
   (`render_preview_image`, `previewImageUrl`). Klaviyo `render_email_template` returns a render
   of unstated type. Resend relies on a shared live editor. Stripo shows the failure mode: the
   agent could not fetch previews. Preview-by-email (Loops preview, Klaviyo preview send job,
   sphericon `test-send`) shows a human, not the agent.
5. **Lint tools the agent calls itself.** Beefree `beefree_check_template` (alt text, contrast,
   links), Unlayer `diagnose_template` (alt text, contrast, image weight, merge tags), Postmark
   `validateTemplate` (syntax, undefined variables), Loops strict validation, Customer.io 422 on
   unknown fields. Beefree's own prompt guide recommends running the check after each section.
6. **Size and Gmail clipping.** Beefree Quality Check defaults to 80 KB HTML for email and warns
   about Gmail clipping; the 102 KB Gmail threshold itself is not from a primary source here (see
   Unverified).
7. **Dark mode.** No vendor page read documents a dark-mode check for agent output. Resend's
   Email Best Practices skill covers "Email design" but its content was not read. Open.
8. **Accessibility basics.** Alt text, contrast, 14px+ body and 44px+ buttons appear in
   Beefree's prompt guide and checker, and in Unlayer `diagnose_template`.
9. **Skills as the vocabulary carrier.** Resend (3 skills), Loops (LMX skill, best-practices
   skill), Customer.io (`cio_skills_*`, served through the MCP itself), Stripo (Agent Plugin),
   Novu. Customer.io is the one that ships skills through the MCP, which matches sphericon's
   playbooks-as-prompts (ADR 0016).
10. **Human editor as the review surface.** Unlayer, Beefree, Klaviyo and Resend all hand the agent's
    result to a visual editor the human opens; Klaviyo's Composer review and Customer.io drafts
    follow the same shape.

## 5. What sphericon lacks today

Grounded in files read in this repo.

- **Body is MJML only** with no structure around it: `ent/schema/email_template.go`
  (`name`, `subject`, `body`; "authored as MJML; compiled to email-safe HTML on send"),
  `docs/adr/0003-templates-copied-not-referenced.md` (Message content = subject + MJML body;
  Template copied into a Broadcast or Automation step, no FK), `GLOSSARY.md` ("Message content").
- **No brand data**: the fixtures in `fixtures/email_templates.yml` are three bare bodies
  (`<mj-text font-size="20px">`, one `<mj-button>`) with no colours, fonts or logo, and no
  Workspace brand field was found in `ent/schema/workspace.go` or `GLOSSARY.md` (searched for
  brand, logo, font, colour).
- **No block vocabulary for agents**: `internal/mcpserver/playbooks/welcome.md` step 4 says only
  "Email bodies are MJML"; the server `instructions` string in `internal/mcpserver/mcpserver.go`
  does not mention MJML at all. Tools are projected from the external OpenAPI
  (`docs/adr/0016-mcp-surface-projection-of-external-api.md`), so a body is an opaque string
  field.
- **No preview or lint back to the agent**: the external contract has `test-send`
  (`typespec/external/resources/broadcasts.tsp`: "Send a rendered preview with sample merge data
  to a single address. Never sends to the audience.") which returns `NoContent`. No operation
  returns rendered HTML, a screenshot or a lint report; the only external "preview" is the
  segment audience preview.
- **MJML errors surface late**: `emailrender.Validate` exists
  (`internal/emailrender/emailrender.go`) but its only non-test caller found is the broadcast send
  planning in `internal/jobs/broadcast.go`, which marks the broadcast `failed` ("A template that
  cannot render would fail every recipient: fail the broadcast once, here"). `validate` in
  `internal/automations/automations.go` checks step types, waits and tags, not email bodies. So
  in what was read, a draft with invalid MJML is accepted and fails when the human schedules it.
  Inference from grep, not an exhaustive trace.
- **Human editor is a plain textarea**: `src/pages/broadcasts/BroadcastForm.tsx` renders the body
  as a Mantine `Textarea`; no visual editor or preview pane was found under `src/pages`. The
  review step for an agent-drafted email is therefore reading MJML or using test-send.
- **Render pipeline** is Liquid, then gomjml, then derived text; tracking and unsubscribe are
  layered after it (`internal/emailrender/emailrender.go` header). That is a clean seam for a
  render/lint tool, since the pipeline already exists and is library-based.
- **Send gating is already strong**: `mcp:send` plus per-operation scopes, send-class tools hidden
  from `tools/list` (ADR 0016; `sendLock` in `internal/mcpserver/mcpserver.go`), which is
  structurally the same as Customer.io's `write` versus `write:live`. Nothing about design needs
  to change that.
- **Untrusted text** is handled for contact-supplied fields (`x-untrusted`, ADR 0016). A brand
  kit fed to an agent would be Workspace-authored, so it sits on the trusted side, but fetched
  URLs (Resend's "give the editor any URL") would be a new untrusted channel.

## 6. Candidate directions and trade-offs

No ranking. Options are not exclusive. "Greenfield" applies (AGENTS.md): no compatibility
shims, so a changed model means changing the contract, schema and callers together.

### A. Brand kit per Workspace

- What: structured Workspace data (colours, fonts, logo, button style, footer) that the render
  pipeline and the MCP expose, as Loops themes/components, Beefree presets, Mailchimp brand kit.
- For: gives the agent facts instead of prompt text; one place to fix a brand; fits the typed,
  governed-field stance of ADR 0006 and ADR 0017 scoping.
- Against: MJML has `mj-attributes`/`mj-style`, so brand could instead live in a base template;
  structured brand needs a rule for how it reaches MJML (injection into the document, Liquid
  variables, or a generated head); touches the "Message content = subject + MJML body" value in
  the glossary and ADR 0003 (copy semantics: does a brand change reach drafts?).

### B. Preview and lint as MCP tools

- What: operations (in the external TypeSpec, so MCP projects them) that return a rendered result
  and a lint report. Vendors differ on form: image (Unlayer), HTML string (Klaviyo-like), a
  report only (Beefree/Postmark/Unlayer `diagnose_template`).
- For: closes the largest gap in section 5; reuses `emailrender` and `Validate`; safe by
  construction because it never sends (unlike `test-send`); lint can run on save to fail drafts
  early.
- Against: an image needs a headless renderer (new dependency and sandbox concern for a
  self-hosted open-core binary); a lint set (alt text, contrast, size, Gmail clipping, dark mode)
  must be chosen and maintained; MCP tool results are currently wrapped for untrusted fields, so
  rendered output with contact data would need care.

### C. Block or template library (constrained vocabulary)

- What: a documented MJML component set the agent must use (shipped playbook or skill text), or
  Keila-style content slots in templates, or Loops-style named components.
- For: cheapest way to raise quality; slots keep header/footer fixed; can ship as text in
  playbooks (ADR 0016 phase 3) with no new infrastructure; suits "simple and powerful, one
  primitive" since it stays MJML.
- Against: a prompt-level vocabulary is advisory, unlike Beefree `permissions` or Loops
  rejection of unknown tags; real enforcement means validating MJML against an allowlist (the
  Mautic plugin does "allowlist validation", per its description); ADR 0003's copy-at-author-time
  rule means a slotted template is copied, so later template fixes do not reach existing drafts.

### D. Visual editor yes or no

- Yes: Resend (React Email editor, open source, embeddable), Beefree, Unlayer, Klaviyo and Stripo
  all treat a human visual editor as the review and repair surface; Keila lists a visual MJML
  editor as in progress (see `docs/research/keila-feature-analysis.md`).
- No: sphericon's MJML-only stance; Resend itself says switching between editor JSON and raw HTML
  is lossy, so a visual editor over MJML needs a round-trip story.
- Middle: a read-only live preview pane next to the textarea (the human sees what the agent
  drafted) without an editing model.
- Cost note: the frontend rule is Mantine only, no custom CSS (AGENTS.md); a third-party editor
  bundle would have to fit that and the AGPL/EE boundary (ADR 0014).

### E. Playbook and server-instructions text only

- What: put the brand facts, the MJML component rules and the lint checklist into the existing
  playbooks and the `instructions` string.
- For: zero schema change; matches how Resend, Loops, Stripo, Customer.io carry knowledge (skills).
- Against: nothing enforces it; no feedback; quality depends on the model.

## 7. Open questions for a grill session

1. Is the unit of design the **Workspace** (one brand) or something finer (a brand per Sending
   domain, per Integration)?
2. Should an agent see a **rendered image**, an HTML string, or only a lint report? Is a headless
   browser acceptable in the self-hosted binary?
3. Is the vocabulary **advisory** (playbook text) or **enforced** (reject MJML outside an
   allowlist, as Loops rejects unknown tags)? Enforced at save, at schedule, or at send?
4. Should MJML validity be checked when a draft is **saved** by an agent rather than only when
   the broadcast is planned to send?
5. Does a brand change propagate to existing templates and drafts (against ADR 0003's copy rule),
   or is brand applied at render time only?
6. Does lint output belong on `/site` for humans too (Beefree Quality Check, Resend editor
   review), or only on MCP?
7. Which checks are in scope: alt text, contrast, size and Gmail clipping, dark mode, broken
   links, merge-tag typos? Who owns the thresholds?
8. Is a **visual editor** in scope for core, for EE, or not at all? What is the human review
   surface for an agent-drafted email beyond reading MJML and `test-send`?
9. Should `test-send` count as send-class over MCP? (In what was read it is a one-address send,
   unlike every vendor's draft-only preview.)
10. May an agent fetch a URL to infer brand (Resend AI editor)? It adds an untrusted input
    channel next to ADR 0016's `x-untrusted` rule.
11. Does an enforced template/slot model fit **automation steps** the same as broadcasts, given
    each step holds its own copy?

## 8. Unverified

Findings that rest on secondary sources, search-snippet summaries, or pages not fully read.

- **Gmail clips messages around 102 KB**: not confirmed from a Google primary source here. Only
  Beefree's own 80 KB check and its "clipped in Gmail" comment were read.
- **Knak test that Claude Design HTML has email-rendering problems**: not located or read;
  dropped.
- **Loops MCP can create, schedule or send campaigns**: the MCP page says "create and manage
  campaigns"; the REST API can schedule a draft campaign; whether the MCP exposes scheduling or
  sending is not stated. MCP tool names not found.
- **Brevo MCP tool names, design vocabulary and send gating**: not found in the page read.
- **Resend MCP send gating beyond `create-broadcast` not sending**: not documented in the pages
  read; Resend's Email Best Practices skill content not read.
- **Resend editor internals**: the tool descriptions refer to "TipTap JSON"; Resend's own prose
  does not name the framework.
- **Stripo REST "Generate an email" endpoint, Mailchimp Transactional MCP, Klaviyo Composer
  Figma grounding, Klaviyo brand tools beyond the tool-list rows**: seen only through search
  summaries or table rows, not read in full.
- **Community MCP servers** (Listmonk, Mautic, MJML): README claims were taken from search
  summaries, not from reading each repo (apart from the Mautic AI Email Sections README).
  `confirmSend`/`dryRun` behaviour is as described by the search result.
- **Beefree MCP access terms**: the FAQ (read) says no approval needed for paying plans; an
  earlier "invite-only beta" statement could not be re-read (page 404).
- **Braze generative email features**: described in the BrazeAI overview; not tested.
  Operator's own email generation docs were not read.
- **Dark mode** support in any competitor's agent tooling: not found either way.
- **Mailchimp/Intuit Assist via API or MCP**: no source found; absence is not proven.

## 9. Sources

Accessed 2026-10-18 unless noted.

Resend

- https://resend.com/blog/react-email-6.md
- https://resend.com/blog/ai-email-editor.md
- https://resend.com/blog/introducing-email-skills.md
- https://resend.com/changelog/introducing-the-new-email-editor.md
- https://resend.com/docs/mcp-server.md
- https://github.com/resend/resend-mcp (README and `src/tools/*.ts` raw files)

Loops

- https://loops.so/docs/mcp-server.md
- https://loops.so/docs/skills
- https://loops.so/docs/creating-emails/lmx.md
- https://loops.so/docs/api-reference/create-campaign.md
- https://loops.so/docs/api-reference/update-campaign.md
- https://loops.so/docs/api-reference/update-email-message.md
- https://loops.so/docs/api-reference/preview-email-message.md
- https://loops.so/docs/llms.txt

Beefree

- https://docs.beefree.io/beefree-sdk/ai-and-mcp/getting-started/mcp-server-tools-and-capabilities.md
- https://docs.beefree.io/beefree-sdk/ai-and-mcp/getting-started/mcp-server-faqs.md
- https://docs.beefree.io/beefree-sdk/ai-and-mcp/getting-started/how-to-get-the-best-ai-output.md
- https://docs.beefree.io/beefree-sdk/ai-and-mcp/brand-rules-for-ai.md
- https://docs.beefree.io/beefree-sdk/qa-tools/quality-check.md
- https://docs.beefree.io/beefree-sdk/sitemap.md

Others

- Unlayer: https://docs.unlayer.com/ai/mcp
- Stripo: https://support.stripo.email/en/articles/15921958-how-to-connect-ai-tools-to-stripo-via-mcp-integration ,
  https://stripo.email/blog/mcp-for-email-marketing-what-you-can-do-with-stripo/ ,
  https://api.stripo.email/reference/generateemail (search result only)
- Klaviyo: https://developers.klaviyo.com/en/docs/klaviyo_mcp_server_available_tools ,
  https://developers.klaviyo.com/en/docs/connect_to_the_klaviyo_mcp_server ,
  https://help.klaviyo.com/hc/en-us/articles/52230280693403 ,
  https://www.klaviyo.com/blog/create-ai-designed-email-templates-you-can-actually-edit
- Mailchimp: https://mailchimp.com/help/create-pre-built-journey-emails-intuit-assist ,
  https://mailchimp.com/developer/marketing/build/ai-tools/mcp-server ,
  https://mailchimp.com/developer/marketing/build/ai-tools
- Customer.io: https://docs.customer.io/ai/mcp/get-started/ , https://docs.customer.io/ai/mcp-server
- Braze: https://braze.com/docs/developer_guide/mcp_server/ ,
  https://braze.com/docs/developer_guide/mcp_server/available_api_functions/ ,
  https://braze.com/docs/developer_guide/mcp_server/usage/ , https://braze.com/docs/user_guide/brazeai/
- Brevo: https://developers.brevo.com/docs/mcp-protocol
- Postmark: https://github.com/ActiveCampaign/postmark-mcp
- Listmonk: https://listmonk.app/docs/apis/templates/ , https://listmonk.app/docs/templating/
- Keila: https://www.keila.io/updates/content-slots/ , https://www.keila.io/updates/improved-previews/
- Mautic (community): https://github.com/Bloomidea/mautic-plugin-ai-email-sections
- Dittofeed: https://github.com/dittofeed/dittofeed
- Novu: https://docs.novu.co/platform/build-with-ai/mcp
- MJML: https://documentation.mjml.io/

Repo files read

- `docs/adr/0003-templates-copied-not-referenced.md`, `docs/adr/0016-mcp-surface-projection-of-external-api.md`
- `fixtures/email_templates.yml`, `ent/schema/email_template.go`, `GLOSSARY.md` (Message content)
- `internal/mcpserver/mcpserver.go`, `internal/mcpserver/playbooks/welcome.md`
- `internal/emailrender/emailrender.go`, `internal/jobs/broadcast.go`, `internal/automations/automations.go`
- `typespec/external/resources/broadcasts.tsp`, `src/pages/broadcasts/BroadcastForm.tsx`
- `docs/research/keila-feature-analysis.md`, `docs/research/mautic-pain-points-analysis.md` (format)
