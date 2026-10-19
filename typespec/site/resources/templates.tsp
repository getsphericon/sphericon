using TypeSpec.Http;
using TypeSpec.Rest;

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

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Site request body for creating a template */
model SiteCreateEmailTemplateInput {
  name: string;
  subject?: string;
  body?: string;
}

/** Site request body for updating a template */
model SiteUpdateEmailTemplateInput {
  name?: string;
  subject?: string;
  body?: string;
}

@tag("Site Templates")
interface SiteTemplates
  extends WorkspaceCrud<
      SiteEmailTemplateResource,
      SiteCreateEmailTemplateInput,
      SiteUpdateEmailTemplateInput
    > {}
