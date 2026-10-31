import "@typespec/http";
import "@typespec/rest";
import "@typespec/openapi";

using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/**
 * Declarative CRUD for a workspace-scoped resource (the Rails `resources`
 * equivalent). Instantiate once per resource; @autoRoute derives the nested
 * path from the resource's @segment and its @parentResource(SiteWorkspaceResource):
 * `/workspaces/{workspaceId}/<segment>` for the collection and
 * `/workspaces/{workspaceId}/<segment>/{id}` for a single item.
 *
 * `WriteConflict` is the extra error a write may raise on a uniqueness clash;
 * it defaults to `never` (no 409). Resources with a unique key (contacts,
 * integrations) instantiate `WorkspaceCrudUnique`, which supplies ConflictProblem.
 */
interface WorkspaceCrudKept<
  Resource extends {},
  CreateInput,
  UpdateInput,
  WriteConflict = never,
  ListFilter extends {} = {}
> {
  /** List resources for the site UI */
  @listsResource(Resource)
  @get
  @autoRoute
  list(...ResourceCollectionParameters<Resource>, ...PageQuery, ...ListFilter):
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
    | BadRequestProblem
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
}

/**
 * Workspace-scoped CRUD of a resource that is archived instead of deleted (ADR 0049): no
 * delete, an `archived` list filter, and `archive` / `unarchive` operations. Archive is
 * refused with 409 while live work uses the resource.
 */
interface WorkspaceCrudArchivable<
  Resource extends {},
  CreateInput,
  UpdateInput,
  WriteConflict = never
>
  extends WorkspaceCrudKept<
      Resource,
      CreateInput,
      UpdateInput,
      WriteConflict,
      ArchivedFilter
    > {
  /** Archive a resource: it leaves the list and new work, keeps its history, and can be restored. Refused with 409 while live work uses it. */
  @action("archive")
  @autoRoute
  archive(...ResourceParameters<Resource>):
    | Resource
    | BadRequestProblem
    | NotFoundProblem
    | InUseProblem;

  /** Restore an archived resource */
  @action("unarchive")
  @autoRoute
  unarchive(...ResourceParameters<Resource>):
    | Resource
    | BadRequestProblem
    | NotFoundProblem;
}

/** Filter of a list of archivable resources */
model ArchivedFilter {
  /** true lists the archived resources instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

interface WorkspaceCrud<
  Resource extends {},
  CreateInput,
  UpdateInput,
  WriteConflict = never,
  DeleteDenied = never,
  ListFilter extends {} = {}
>
  extends WorkspaceCrudKept<
      Resource,
      CreateInput,
      UpdateInput,
      WriteConflict,
      ListFilter
    > {
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
  DeleteDenied = never,
  ListFilter extends {} = {}
>
  extends WorkspaceCrud<
      Resource,
      CreateInput,
      UpdateInput,
      ConflictProblem,
      DeleteDenied,
      ListFilter
    > {}

/**
 * Flat, token-scoped CRUD for the external (public) API. No parent nesting —
 * `/<segment>` and `/<segment>/{id}`. Every operation can return 401
 * (bearer-token auth) and 403 (a token without the scope the operation declares,
 * ADR 0061). `ReadScope` gates list and get, `WriteScope` create and update.
 * `WriteConflict` defaults to `never` (no 409); resources with a unique key use
 * `ExternalCrudUnique`.
 */
interface ExternalCrudKept<
  Resource extends {},
  CreateInput,
  UpdateInput,
  ReadScope extends valueof ApiTokenScope,
  WriteScope extends valueof ApiTokenScope,
  WriteConflict = never,
  ListFilter extends {} = {}
> {
  /** List resources with pagination */
  @listsResource(Resource)
  @get
  @autoRoute
  @requiresScopes(#[ReadScope])
  list(...ResourceCollectionParameters<Resource>, ...PageQuery, ...ListFilter):
    | Page<Resource>
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;

  /** Create a resource */
  @createsResource(Resource)
  @post
  @autoRoute
  @requiresScopes(#[WriteScope])
  create(...ResourceCollectionParameters<Resource>, @body body: CreateInput):
    | {
        @statusCode statusCode: 201;
        @body body: Resource;
      }
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | WriteConflict
    | UnprocessableEntityProblem;

  /** Get a resource by ID */
  @readsResource(Resource)
  @get
  @autoRoute
  @requiresScopes(#[ReadScope])
  get(...ResourceParameters<Resource>):
    | Resource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Update a resource */
  @updatesResource(Resource)
  @put
  @autoRoute
  @requiresScopes(#[WriteScope])
  update(...ResourceParameters<Resource>, @body body: UpdateInput):
    | Resource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | WriteConflict
    | UnprocessableEntityProblem;
}

/** Flat external CRUD of a resource that is archived instead of deleted (ADR 0049).
 * Archiving and restoring need `WriteScope`. */
interface ExternalCrudArchivable<
  Resource extends {},
  CreateInput,
  UpdateInput,
  ReadScope extends valueof ApiTokenScope,
  WriteScope extends valueof ApiTokenScope,
  WriteConflict = never
>
  extends ExternalCrudKept<
      Resource,
      CreateInput,
      UpdateInput,
      ReadScope,
      WriteScope,
      WriteConflict,
      ArchivedFilter
    > {
  /** Archive a resource: it leaves the list and new work, keeps its history, and can be restored. Refused with 409 while live work uses it. */
  @action("archive")
  @autoRoute
  @requiresScopes(#[WriteScope])
  archive(...ResourceParameters<Resource>):
    | Resource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | InUseProblem;

  /** Restore an archived resource */
  @action("unarchive")
  @autoRoute
  @requiresScopes(#[WriteScope])
  unarchive(...ResourceParameters<Resource>):
    | Resource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
}

/** Flat external CRUD with a delete, which needs `DeleteScope` (the write scope of most
 * resources; deleting a Contact erases it and needs contacts:erase). */
interface ExternalCrud<
  Resource extends {},
  CreateInput,
  UpdateInput,
  ReadScope extends valueof ApiTokenScope,
  WriteScope extends valueof ApiTokenScope,
  DeleteScope extends valueof ApiTokenScope,
  WriteConflict = never,
  DeleteDenied = never,
  ListFilter extends {} = {}
>
  extends ExternalCrudKept<
      Resource,
      CreateInput,
      UpdateInput,
      ReadScope,
      WriteScope,
      WriteConflict,
      ListFilter
    > {
  /** Delete a resource */
  @deletesResource(Resource)
  @delete
  @autoRoute
  @requiresScopes(#[DeleteScope])
  delete(...ResourceParameters<Resource>):
    | NoContentResponse
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | UnprocessableEntityProblem
    | DeleteDenied;
}

/** Flat external CRUD for a resource with a unique key (adds 409 on writes). */
interface ExternalCrudUnique<
  Resource extends {},
  CreateInput,
  UpdateInput,
  ReadScope extends valueof ApiTokenScope,
  WriteScope extends valueof ApiTokenScope,
  DeleteScope extends valueof ApiTokenScope,
  DeleteDenied = never
>
  extends ExternalCrud<
      Resource,
      CreateInput,
      UpdateInput,
      ReadScope,
      WriteScope,
      DeleteScope,
      ConflictProblem,
      DeleteDenied
    > {}
