package scopegen

import (
	"fmt"
	"strings"
)

const (
	// workspaceParam is the path parameter that addresses a Workspace on /site.
	workspaceParam = "workspaceId"
	// permissionsExtension is where an operation declares the Permissions it needs: on /site its
	// caller's, on /api a Connection's User's.
	permissionsExtension = "x-required-permissions"
	// withheldExtension marks the operation a User withheld by the Two-factor requirement may
	// still call in the Workspace (ADR 0020).
	withheldExtension = "x-allowed-while-withheld"
)

// SitePermissions is the /site operation-to-access map (ADR 0061). An operation whose path has a
// workspaceId declares the Permissions it needs (an empty list: any member); one without
// declares none and is not marked as allowed while withheld.
var SitePermissions = Table{
	Spec:    "openapi/site.openapi.json",
	Out:     "internal/api/site/access_gen.go",
	Package: "site",
	Imports: []string{
		`siteapi "github.com/getsphericon/sphericon/gen/site"`,
		`"github.com/getsphericon/sphericon/internal/accounts"`,
	},
	Doc: []string{
		"operationAccess is what each operation of /site declares in the contract: whether it",
		"addresses a Workspace, the Permissions its caller's Role must hold there, and whether the",
		"Two-factor requirement lets a withheld caller through. An operation with no entry is",
		"refused to everyone.",
	},
	Var:  "operationAccess",
	Type: "map[siteapi.OperationName]access",
	Rows: PerOperation(siteAccess),
}

// siteAccess is the row of one /site operation.
func siteAccess(op Operation) (string, error) {
	perms, declared, err := op.Strings(permissionsExtension)
	if err != nil {
		return "", err
	}
	withheld, err := op.Flag(withheldExtension)
	if err != nil {
		return "", err
	}
	key := "siteapi." + op.Name() + "Operation"
	if !op.HasPathParam(workspaceParam) {
		if declared {
			return "", fmt.Errorf("%s declares %s but has no %s in its path: only an operation of a Workspace declares Permissions", op, permissionsExtension, workspaceParam)
		}
		if withheld {
			return "", fmt.Errorf("%s declares %s but has no %s in its path: only an operation of a Workspace is withheld", op, withheldExtension, workspaceParam)
		}
		return key + ": {}", nil
	}
	if !declared {
		return "", fmt.Errorf("%s declares no %s: declare its Permissions in TypeSpec, an empty list for any member", op, permissionsExtension)
	}
	fields := []string{"workspace: true"}
	if len(perms) > 0 {
		fields = append(fields, "permissions: []accounts.Permission"+names("accounts.Perm", perms))
	}
	if withheld {
		fields = append(fields, "whileWithheld: true")
	}
	return key + ": {" + strings.Join(fields, ", ") + "}", nil
}
