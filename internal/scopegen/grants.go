package scopegen

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// grantExtension is the extension on a grantable scope enum that gives each of its members the
// Permission a User must hold to grant it to a Connection.
const grantExtension = "x-grant-permissions"

// grantableEnums are the scope enums an OAuth client can be granted (typespec/common/scopes.tsp);
// each must declare grantExtension, and no other schema may.
var grantableEnums = []string{"GrantableScope", "SendScope", "SetupScope"}

// scopeRule is one row of grantExtension: a scope and the Permission granting it needs, "" for
// none.
type scopeRule struct {
	Scope      string `json:"scope"`
	Permission string `json:"permission"`
}

// GrantPermissions is the map from a grantable scope to the Permission a User must hold to grant
// it (ADR 0061, "A Connection belongs to a User"), from the `x-grant-permissions` each grantable
// scope enum declares in TypeSpec. Every member of those enums must be listed once, with a
// Permission of the contract's Permission enum or none; a missing, unknown or misplaced
// declaration fails generation. The map holds the scopes that need a Permission.
var GrantPermissions = Table{
	Spec:    "openapi/site.openapi.json",
	Out:     "internal/accounts/grant_permissions_gen.go",
	Package: "accounts",
	Doc: []string{
		"GrantPermissions names the Permission a User must hold to grant a scope to a Connection, as",
		"the contract declares it on each grantable scope (ADR 0061). A grantable scope absent here is",
		"any member's to grant.",
	},
	Var:  "GrantPermissions",
	Type: "map[string]Permission",
	Rows: grantRows,
}

func grantRows(doc Document) ([]string, error) {
	schemas, err := enumSchemas(doc)
	if err != nil {
		return nil, err
	}
	permissions := schemas["Permission"].values()
	if len(permissions) == 0 {
		return nil, errors.New("the document has no Permission enum")
	}
	for name, schema := range schemas {
		if schema.Grants != nil && !slices.Contains(grantableEnums, name) {
			return nil, fmt.Errorf("%s declares %s, which only a grantable scope enum (%s) may", name, grantExtension, strings.Join(grantableEnums, ", "))
		}
	}
	var rows []string
	for _, name := range grantableEnums {
		enum, ok := schemas[name]
		if !ok {
			return nil, fmt.Errorf("the document has no %s enum", name)
		}
		if enum.Grants == nil {
			return nil, fmt.Errorf("%s declares no %s: list each of its scopes in TypeSpec, with the Permission granting it needs or none", name, grantExtension)
		}
		listed := make([]string, 0, len(*enum.Grants))
		for _, rule := range *enum.Grants {
			if !slices.Contains(enum.values(), rule.Scope) {
				return nil, fmt.Errorf("%s %s lists %q, which is no %s", name, grantExtension, rule.Scope, name)
			}
			if slices.Contains(listed, rule.Scope) {
				return nil, fmt.Errorf("%s %s lists %q twice", name, grantExtension, rule.Scope)
			}
			listed = append(listed, rule.Scope)
			if rule.Permission == "" {
				continue
			}
			if !slices.Contains(permissions, rule.Permission) {
				return nil, fmt.Errorf("%s %s gives %q the permission %q, which is no Permission", name, grantExtension, rule.Scope, rule.Permission)
			}
			rows = append(rows, fmt.Sprintf("%q: Perm%s", rule.Scope, valueName(rule.Permission)))
		}
		for _, scope := range enum.values() {
			if !slices.Contains(listed, scope) {
				return nil, fmt.Errorf("%s %s does not list %q: declare the Permission granting it needs, or none", name, grantExtension, scope)
			}
		}
	}
	return rows, nil
}
