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

  /** Value type: inferred from the first value, changed by an owner or admin */
  type: SiteCustomFieldType;

  /** The operators a rule leaf on this field takes, by its type */
  ruleOperators: SiteConditionOperator[];

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Request body for updating a custom field: its key never changes */
model SiteUpdateCustomFieldInput {
  /** Display name */
  @minLength(1)
  name?: string;

  /** Value type. Stored values are not rewritten: the leaves of the new type read them as they are */
  type?: SiteCustomFieldType;
}

// The catalogue: created by use, listed, renamed and retyped. Nested via @parentResource + @autoRoute.
@tag("Site Custom Fields")
namespace SiteCustomFields {
  /** List the workspace's custom field definitions */
  @listsResource(SiteCustomFieldResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  op list(...ResourceCollectionParameters<SiteCustomFieldResource>):
    | Page<SiteCustomFieldResource>
    | BadRequestProblem
    | NotFoundProblem;

  /** Rename a custom field or change its type. Refused with 409 while a segment, a rule or a scheduled broadcast tests the field with an operator the new type does not take (`usedBy` names them). Owners and admins only. */
  @updatesResource(SiteCustomFieldResource)
  @put
  @autoRoute
  @requiresPermissions(#[Permission.customFieldsManage])
  op update(
    ...ResourceParameters<SiteCustomFieldResource>,
    @body body: SiteUpdateCustomFieldInput,
  ):
    | SiteCustomFieldResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | InUseProblem
    | UnprocessableEntityProblem;
}
