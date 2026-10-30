package scopegen

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// ExternalScopes is the external API's operation-to-scopes map (ADR 0061): every operation
// declares `x-required-scopes`, an empty list being one any valid token may call, and every scope
// it names must be an ApiTokenScope.
var ExternalScopes = Table{
	Spec:    "openapi/external.openapi.json",
	Out:     "internal/api/auth/external_scopes_gen.go",
	Package: "auth",
	Imports: []string{
		`externalapi "github.com/getsphericon/sphericon/gen/external"`,
		`"github.com/getsphericon/sphericon/internal/apitokens"`,
	},
	Doc: []string{
		"ExternalOperationScopes names the scopes each operation of the external API needs, as the",
		"contract declares them (ADR 0061): an API token must carry all of them. An operation with no",
		"entry is refused to every token; an entry with no scopes is open to any valid token.",
	},
	Var:  "ExternalOperationScopes",
	Type: "map[externalapi.OperationName][]apitokens.Scope",
	Rows: externalRows,
}

// externalRows checks every scope an operation declares against the ApiTokenScope enum.
func externalRows(doc Document) ([]string, error) {
	schemas, err := enumSchemas(doc)
	if err != nil {
		return nil, err
	}
	known := schemas["ApiTokenScope"].values()
	if len(known) == 0 {
		return nil, errors.New("the document has no ApiTokenScope enum")
	}
	return PerOperation(func(op Operation) (string, error) {
		scopes, declared, err := op.Strings(scopesExtension)
		if err != nil {
			return "", err
		}
		if !declared {
			return "", fmt.Errorf("%s declares no %s: declare its scopes in TypeSpec, an empty list for none", op, scopesExtension)
		}
		for _, s := range scopes {
			if !slices.Contains(known, s) {
				return "", fmt.Errorf("%s requires %q, which is no ApiTokenScope", op, s)
			}
		}
		return fmt.Sprintf("externalapi.%sOperation: %s", op.Name(), names("apitokens.Scope", scopes)), nil
	})(doc)
}

// ExternalPermissions is the map from an external operation to the Permissions a Connection's User
// must hold in their current Role to call it (ADR 0061: a Connection can do what its scopes and
// its User's Role allow). An operation that mirrors a /site action gated by a Permission its
// scopes do not already need to be granted declares `x-required-permissions`; one that declares
// none has no entry. Every Permission named must be in the contract's Permission enum.
var ExternalPermissions = Table{
	Spec:    "openapi/external.openapi.json",
	Out:     "internal/api/auth/external_permissions_gen.go",
	Package: "auth",
	Imports: []string{
		`externalapi "github.com/getsphericon/sphericon/gen/external"`,
		`"github.com/getsphericon/sphericon/internal/accounts"`,
	},
	Doc: []string{
		"ExternalOperationPermissions names the Permissions a Connection's User must hold in their",
		"current Role to call an operation of the external API, as the contract declares them (ADR",
		"0061). An API token of the Workspace has no User and skips them; an operation with no entry",
		"needs none beyond its scopes.",
	},
	Var:  "ExternalOperationPermissions",
	Type: "map[externalapi.OperationName][]accounts.Permission",
	Rows: externalPermissionRows,
}

// externalPermissionRows checks every Permission an operation declares against the Permission enum.
func externalPermissionRows(doc Document) ([]string, error) {
	schemas, err := enumSchemas(doc)
	if err != nil {
		return nil, err
	}
	known := schemas["Permission"].values()
	if len(known) == 0 {
		return nil, errors.New("the document has no Permission enum")
	}
	var rows []string
	for _, op := range doc.Operations {
		perms, _, err := op.Strings(permissionsExtension)
		if err != nil {
			return nil, err
		}
		for _, p := range perms {
			if !slices.Contains(known, p) {
				return nil, fmt.Errorf("%s requires %q, which is no Permission", op, p)
			}
		}
		if len(perms) > 0 {
			rows = append(rows, fmt.Sprintf("externalapi.%sOperation: %s", op.Name(), names("accounts.Perm", perms)))
		}
	}
	return rows, nil
}

// enumSchema is a schema of the contract as the tables read it: the values of an enum and the
// extension a grantable scope enum carries. Any other schema decodes to neither.
type enumSchema struct {
	Enum   []any        `json:"enum"`
	Grants *[]scopeRule `json:"x-grant-permissions"`
}

// values are the string values of the enum.
func (s enumSchema) values() []string {
	var out []string
	for _, v := range s.Enum {
		if str, ok := v.(string); ok {
			out = append(out, str)
		}
	}
	return out
}

// enumSchemas are the component schemas of the document, by name.
func enumSchemas(doc Document) (map[string]enumSchema, error) {
	var raw struct {
		Components struct {
			Schemas map[string]enumSchema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(doc.Raw, &raw); err != nil {
		return nil, err
	}
	return raw.Components.Schemas, nil
}
