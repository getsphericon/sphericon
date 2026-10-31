// @grantable and @grantedWith.
import "../../common/scopes.tsp";

using Sphericon;

namespace Fixture;

@grantable
enum Grantable {
  read: "things:read",

  @grantedWith(Permission.webhooksManage)
  write: "things:write",
}

/** Spreading a @grantable enum copies its members and their marks, which is no misuse */
enum Every {
  ...Grantable,
  other: "other:read",
}

enum Plain {
  // expect-next-line: sphericon/granted-outside-grantable
  @grantedWith(Permission.auditRead)
  read: "plain:read",
}
