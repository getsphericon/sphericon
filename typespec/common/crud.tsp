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
 * `/workspaces/{workspaceId}/<segment>/{id}` for a single item. Every operation answers a
 * refusal with the `Problem` default response, whatever its status.
 */
interface WorkspaceCrudKept<
  Resource extends {},
  CreateInput,
  UpdateInput,
  ListFilter extends {} = {}
> {
  /** List resources for the site UI */
  @listsResource(Resource)
  @get
  @autoRoute
  list(...ResourceCollectionParameters<Resource>, ...PageQuery, ...ListFilter):
    | Page<Resource>
    | Problem;

  /** Create a resource from the site UI */
  @createsResource(Resource)
  @post
  @autoRoute
  create(...ResourceCollectionParameters<Resource>, @body body: CreateInput):
    | {
        @statusCode statusCode: 201;
        @body body: Resource;
      }
    | Problem;

  /** Get a resource by ID for the site UI */
  @readsResource(Resource)
  @get
  @autoRoute
  get(...ResourceParameters<Resource>): Resource | Problem;

  /** Update a resource from the site UI */
  @updatesResource(Resource)
  @put
  @autoRoute
  update(...ResourceParameters<Resource>, @body body: UpdateInput):
    | Resource
    | Problem;
}

/**
 * Workspace-scoped CRUD of a resource that is archived instead of deleted (ADR 0100): no
 * delete, an `archived` list filter, and `archive` / `unarchive` operations. Archive is
 * refused with 409 while live work uses the resource, and an update of an archived resource
 * with 409 until it is unarchived.
 */
interface WorkspaceCrudArchivable<Resource extends {}, CreateInput, UpdateInput>
  extends WorkspaceCrudKept<
      Resource,
      CreateInput,
      UpdateInput,
      ArchivedFilter
    > {
  /** Archive a resource: it leaves the list and new work, keeps its history, and can be restored. Refused with 409 while live work uses it. */
  @action("archive")
  @autoRoute
  archive(...ResourceParameters<Resource>): Resource | Problem;

  /** Restore an archived resource */
  @action("unarchive")
  @autoRoute
  unarchive(...ResourceParameters<Resource>): Resource | Problem;
}

/** Filter of a list of archivable resources */
model ArchivedFilter {
  /** true lists the archived resources instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

/** Workspace-scoped CRUD with a delete. */
interface WorkspaceCrud<
  Resource extends {},
  CreateInput,
  UpdateInput,
  ListFilter extends {} = {}
> extends WorkspaceCrudKept<Resource, CreateInput, UpdateInput, ListFilter> {
  /** Delete a resource from the site UI */
  @deletesResource(Resource)
  @delete
  @autoRoute
  delete(...ResourceParameters<Resource>): NoContentResponse | Problem;
}

/**
 * Flat, token-scoped CRUD for the external (public) API. No parent nesting —
 * `/<segment>` and `/<segment>/{id}`. Every operation can be refused with 401
 * (bearer-token auth) and 403 (a token without the scope the operation declares,
 * ADR 0061), answered with the `Problem` default response. `ReadScope` gates list and get,
 * `WriteScope` create and update.
 */
interface ExternalCrudKept<
  Resource extends {},
  CreateInput,
  UpdateInput,
  ReadScope extends valueof ApiTokenScope,
  WriteScope extends valueof ApiTokenScope,
  ListFilter extends {} = {}
> {
  /** List resources with pagination */
  @listsResource(Resource)
  @get
  @autoRoute
  @requiresScopes(#[ReadScope])
  list(...ResourceCollectionParameters<Resource>, ...PageQuery, ...ListFilter):
    | Page<Resource>
    | Problem;

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
    | Problem;

  /** Get a resource by ID */
  @readsResource(Resource)
  @get
  @autoRoute
  @requiresScopes(#[ReadScope])
  get(...ResourceParameters<Resource>): Resource | Problem;

  /** Update a resource */
  @updatesResource(Resource)
  @put
  @autoRoute
  @requiresScopes(#[WriteScope])
  update(...ResourceParameters<Resource>, @body body: UpdateInput):
    | Resource
    | Problem;
}

/** Flat external CRUD of a resource that is archived instead of deleted (ADR 0100).
 * Archiving and restoring need `WriteScope`; an update of an archived resource is refused with
 * 409 until it is unarchived. */
interface ExternalCrudArchivable<
  Resource extends {},
  CreateInput,
  UpdateInput,
  ReadScope extends valueof ApiTokenScope,
  WriteScope extends valueof ApiTokenScope
>
  extends ExternalCrudKept<
      Resource,
      CreateInput,
      UpdateInput,
      ReadScope,
      WriteScope,
      ArchivedFilter
    > {
  /** Archive a resource: it leaves the list and new work, keeps its history, and can be restored. Refused with 409 while live work uses it. */
  @action("archive")
  @autoRoute
  @requiresScopes(#[WriteScope])
  archive(...ResourceParameters<Resource>): Resource | Problem;

  /** Restore an archived resource */
  @action("unarchive")
  @autoRoute
  @requiresScopes(#[WriteScope])
  unarchive(...ResourceParameters<Resource>): Resource | Problem;
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
  ListFilter extends {} = {}
>
  extends ExternalCrudKept<
      Resource,
      CreateInput,
      UpdateInput,
      ReadScope,
      WriteScope,
      ListFilter
    > {
  /** Delete a resource */
  @deletesResource(Resource)
  @delete
  @autoRoute
  @requiresScopes(#[DeleteScope])
  delete(...ResourceParameters<Resource>): NoContentResponse | Problem;
}
