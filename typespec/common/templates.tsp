// The Template write bodies: /site and /api take the same input.

namespace Sphericon;

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
  doc?: EditorDocument;
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
  doc?: EditorDocument;
}
