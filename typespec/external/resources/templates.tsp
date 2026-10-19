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

/** Request body for creating a template */
model CreateTemplateInput {
  /** Template name */
  name: string;

  /** Default subject line */
  subject?: string;

  /** HTML body with Liquid merge tags. Write this or `doc`, not both. */
  body?: string;

  /** The editor's JSON document, an opaque object checked by the server against the schema `email_schema` returns. The server renders the
   * body from it in the same save; a document that cannot be rendered is a 422 with the reason and nothing is stored. */
  doc?: Record<unknown>;
}

/** Request body for updating a template */
model UpdateTemplateInput {
  /** Template name */
  name?: string;

  /** Default subject line */
  subject?: string;

  /** HTML body. Writing only a body clears the stored document. Write this or `doc`, not both. */
  body?: string;

  /** The editor's JSON document, an opaque object checked by the server against the schema `email_schema` returns. The server renders the
   * body from it in the same save; a document that cannot be rendered is a 422 with the reason and nothing is stored. */
  doc?: Record<unknown>;
}

@tag("Templates")
interface Templates
  extends ExternalCrud<
      TemplateResource,
      CreateTemplateInput,
      UpdateTemplateInput
    > {}
