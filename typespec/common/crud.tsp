import "@typespec/http";
import "@typespec/rest";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

namespace Sphericon;

/**
 * Declarative CRUD for a workspace-scoped resource (the Rails `resources`
 * equivalent). Instantiate once per resource; @autoRoute derives the nested
 * path from the resource's @segment and its @parentResource(SiteWorkspaceResource):
 * `/workspaces/{slug}/<segment>` for the collection and
 * `/workspaces/{slug}/<segment>/{id}` for a single item.
 *
 * `WriteConflict` is the extra error a write may raise on a uniqueness clash;
 * it defaults to `never` (no 409). Resources with a unique key (contacts,
 * integrations) instantiate `WorkspaceCrudUnique`, which supplies ConflictProblem.
 */
interface WorkspaceCrud<
  Resource extends {},
  CreateInput,
  UpdateInput,
  WriteConflict = never,
  DeleteDenied = never
> {
  /** List resources for the site UI */
  @listsResource(Resource)
  @get
  @autoRoute
  list(...ResourceCollectionParameters<Resource>, ...PageQuery):
    | Page<Resource>
    | BadRequestProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;

  /** Create a resource from the site UI */
  @createsResource(Resource)
  @post
  @autoRoute
  create(...ResourceCollectionParameters<Resource>, @body body: CreateInput):
    | {
        @statusCode statusCode: 201;
        @body body: Resource;
      }
    | NotFoundProblem
    | WriteConflict
    | UnprocessableEntityProblem;

  /** Get a resource by ID for the site UI */
  @readsResource(Resource)
  @get
  @autoRoute
  get(...ResourceParameters<Resource>):
    | Resource
    | BadRequestProblem
    | NotFoundProblem;

  /** Update a resource from the site UI */
  @updatesResource(Resource)
  @put
  @autoRoute
  update(...ResourceParameters<Resource>, @body body: UpdateInput):
    | Resource
    | BadRequestProblem
    | NotFoundProblem
    | WriteConflict
    | UnprocessableEntityProblem;

  /** Delete a resource from the site UI */
  @deletesResource(Resource)
  @delete
  @autoRoute
  delete(...ResourceParameters<Resource>):
    | NoContentResponse
    | BadRequestProblem
    | NotFoundProblem
    | DeleteDenied;
}

/**
 * Workspace-scoped CRUD for a resource with a unique key (adds 409 on writes).
 * `DeleteDenied` is the extra error a delete may raise (e.g. 403 for a role gate).
 */
interface WorkspaceCrudUnique<
  Resource extends {},
  CreateInput,
  UpdateInput,
  DeleteDenied = never
>
  extends WorkspaceCrud<
      Resource,
      CreateInput,
      UpdateInput,
      ConflictProblem,
      DeleteDenied
    > {}

/**
 * Flat, token-scoped CRUD for the external (public) API. No parent nesting —
 * `/<segment>` and `/<segment>/{id}`. Every operation can return 401
 * (bearer-token auth). `WriteConflict` defaults to `never` (no 409); resources
 * with a unique key use `ExternalCrudUnique`.
 */
interface ExternalCrud<
  Resource extends {},
  CreateInput,
  UpdateInput,
  WriteConflict = never
> {
  /** List resources with pagination */
  @listsResource(Resource)
  @get
  @autoRoute
  list(...ResourceCollectionParameters<Resource>, ...PageQuery):
    | Page<Resource>
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;

  /** Create a resource */
  @createsResource(Resource)
  @post
  @autoRoute
  create(...ResourceCollectionParameters<Resource>, @body body: CreateInput):
    | {
        @statusCode statusCode: 201;
        @body body: Resource;
      }
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | WriteConflict
    | UnprocessableEntityProblem;

  /** Get a resource by ID */
  @readsResource(Resource)
  @get
  @autoRoute
  get(...ResourceParameters<Resource>):
    | Resource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Update a resource */
  @updatesResource(Resource)
  @put
  @autoRoute
  update(...ResourceParameters<Resource>, @body body: UpdateInput):
    | Resource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | WriteConflict
    | UnprocessableEntityProblem;

  /** Delete a resource */
  @deletesResource(Resource)
  @delete
  @autoRoute
  delete(...ResourceParameters<Resource>):
    | NoContentResponse
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem;
}

/** Flat external CRUD for a resource with a unique key (adds 409 on writes). */
interface ExternalCrudUnique<Resource extends {}, CreateInput, UpdateInput>
  extends ExternalCrud<Resource, CreateInput, UpdateInput, ConflictProblem> {}
