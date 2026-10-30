using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A scope a Service credential may carry */
enum ServiceScope {
  /** Read the Usage of Workspaces */
  usageRead: "usage:read",

  /** Read the Workspaces and their owners */
  workspacesRead: "workspaces:read",

  /** Read the Users and their consent to the operator's mailings */
  usersRead: "users:read",

  /** Read the verified Tracking domain hosts */
  domainsRead: "domains:read",

  /** Set and clear Staff holds */
  holdsWrite: "holds:write",
}

/** What a Service credential is: its name, its scopes and its expiry */
model ServiceIdentityResource {
  /** The service's name, as recorded in the Audit log */
  name: string;

  /** The scopes the credential carries */
  scopes: ServiceScope[];

  /** When the credential stops working; absent when it never expires */
  expiresAt?: Timestamp | null;
}

@tag("Service Identity")
@route("/identity")
namespace ServiceIdentity {
  /** The calling credential's own name, scopes and expiry */
  @get
  @extension("x-required-scopes", #[])
  op get():
    | ServiceIdentityResource
    | UnauthorizedProblem
    | TooManyRequestsProblem;
}
