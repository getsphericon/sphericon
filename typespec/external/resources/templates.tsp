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

  /** HTML body with Liquid merge tags */
  body?: string;
}

/** Request body for updating a template */
model UpdateTemplateInput {
  /** Template name */
  name?: string;

  /** Default subject line */
  subject?: string;

  /** HTML body with Liquid merge tags */
  body?: string;
}

@tag("Templates")
interface Templates
  extends ExternalCrud<
      TemplateResource,
      CreateTemplateInput,
      UpdateTemplateInput
    > {}
