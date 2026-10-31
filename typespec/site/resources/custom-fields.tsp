using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A Custom field's value type */
enum SiteCustomFieldType {
  string,
  number,
  bool,
  datetime,
}

/** Custom field definition — a typed, named Contact attribute (ADR 0006). Auto-created
 on first sight from Identify; the catalogue feeds the segment builder. */
@resource("custom-fields")
@segment("custom-fields")
@parentResource(SiteWorkspaceResource)
model SiteCustomFieldResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Machine key — how values are addressed in custom fields and segment conditions */
  key: string;

  /** Display name (renameable) */
  name: string;

  /** Inferred value type */
  type: SiteCustomFieldType;

  /** The operators a rule leaf on this field takes, by its type */
  ruleOperators: SiteConditionOperator[];

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

// Read-only catalogue: list only. Nested via @parentResource + @autoRoute.
@tag("Site Custom Fields")
namespace SiteCustomFields {
  /** List the workspace's custom field definitions */
  @listsResource(SiteCustomFieldResource)
  @get
  @autoRoute
  @extension("x-required-permissions", #[])
  op list(...ResourceCollectionParameters<SiteCustomFieldResource>):
    | Page<SiteCustomFieldResource>
    | BadRequestProblem
    | NotFoundProblem;
}
