using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Reusable email template used by the site UI */
@resource("templates")
@segment("templates")
@parentResource(SiteWorkspaceResource)
model SiteEmailTemplateResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Template name */
  name: string;

  /** Default subject line */
  subject: string;

  /** HTML body with Liquid merge tags */
  body: string;

  /** The editor's JSON document the body was built from; null when the body was written as HTML */
  doc?: Record<unknown> | null;

  /** Version of the editor that rendered the document; null with it */
  editorVersion?: string | null;

  /** Warnings of the lint over the stored content (ADR 0040); set on the response of a save, never blocks it */
  warnings?: LintFinding[];

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for creating a template */
model SiteCreateEmailTemplateInput {
  name: string;
  subject?: string;

  /** HTML body with Liquid merge tags. Write this or `doc`, not both. */
  body?: string;

  /** The editor's JSON document, an opaque object checked by the server. The server renders the
   * body from it in the same save; a document that cannot be rendered is a 422. */
  doc?: Record<unknown>;
}

/** Site request body for updating a template */
model SiteUpdateEmailTemplateInput {
  name?: string;
  subject?: string;

  /** HTML body. Writing only a body clears the stored document. Write this or `doc`, not both. */
  body?: string;

  /** The editor's JSON document, an opaque object checked by the server. The server renders the
   * body from it in the same save; a document that cannot be rendered is a 422. */
  doc?: Record<unknown>;
}

@tag("Site Templates")
interface SiteTemplates
  extends WorkspaceCrud<
      SiteEmailTemplateResource,
      SiteCreateEmailTemplateInput,
      SiteUpdateEmailTemplateInput
    > {}

@@extension(SiteTemplates.create, "x-required-permissions", #[]);
@@extension(SiteTemplates.delete, "x-required-permissions", #[]);
@@extension(SiteTemplates.get, "x-required-permissions", #[]);
@@extension(SiteTemplates.list, "x-required-permissions", #[]);
@@extension(SiteTemplates.update, "x-required-permissions", #[]);
