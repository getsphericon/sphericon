using TypeSpec.Http;
using TypeSpec.Rest;

namespace Sphericon;

/** Reusable email template */
@resource("templates")
@segment("templates")
model TemplateResource {
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

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

@tag("Templates")
interface Templates
  extends ExternalCrud<
      TemplateResource,
      CreateTemplateInput,
      UpdateTemplateInput,
      ApiTokenScope.templatesRead,
      ApiTokenScope.templatesWrite,
      ApiTokenScope.templatesWrite
    > {}
