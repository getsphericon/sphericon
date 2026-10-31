// The decorators typed by a contract enum take only its members, and each is declared once. These
// fail the checker, which stops compilation before the checks of $onValidate.
import "../../common/scopes.tsp";

using Sphericon;

namespace Fixture;

op permissionsOfAString(): void;
op scopesOfAPermission(): void;
op permissionsTwice(): void;

// expect-next-line: invalid-argument
@@requiresPermissions(permissionsOfAString, #["contacts:export"]);

// expect-next-line: invalid-argument
@@requiresScopes(scopesOfAPermission, #[Permission.auditRead]);

@@requiresPermissions(permissionsTwice, #[]);

// expect-next-line: sphericon/declared-twice
@@requiresPermissions(permissionsTwice, #[Permission.auditRead]);
