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

@tag("Site Templates")
interface SiteTemplates
  extends WorkspaceCrud<
      SiteEmailTemplateResource,
      CreateTemplateInput,
      UpdateTemplateInput
    > {}

@@extension(SiteTemplates.create, "x-required-permissions", #[]);
@@extension(SiteTemplates.delete, "x-required-permissions", #[]);
@@extension(SiteTemplates.get, "x-required-permissions", #[]);
@@extension(SiteTemplates.list, "x-required-permissions", #[]);
@@extension(SiteTemplates.update, "x-required-permissions", #[]);
