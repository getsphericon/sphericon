// `@everyOperationDeclares("workspacePermissions")`, as /site declares it.
import "@typespec/http";
import "../../common/permissions.tsp";

using TypeSpec.Http;
using Sphericon;

@service(#{ title: "Fixture" })
@everyOperationDeclares("workspacePermissions")
namespace Fixture;

@route("/workspaces/{workspaceId}/members")
@get
@requiresPermissions(#[Permission.membersManage])
op members(@path workspaceId: int64): void;

@route("/workspaces/{workspaceId}/contacts")
@get
@requiresPermissions(#[])
@allowedWhileWithheld
op contacts(@path workspaceId: int64): void;

@route("/me")
@get
@allowedWhileAcceptancePending
op me(): void;

// expect-next-line: sphericon/permissions-missing
@route("/workspaces/{workspaceId}/tags")
@get
op tags(@path workspaceId: int64): void;

// expect-next-line: sphericon/outside-workspace
@route("/account")
@get
@requiresPermissions(#[])
op account(): void;

// expect-next-line: sphericon/outside-workspace
@route("/session")
@get
@allowedWhileWithheld
op session(): void;
