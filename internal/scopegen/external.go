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
	Imports: []string{`externalapi "github.com/getsphericon/sphericon/gen/external"`},
	Doc: []string{
		"ExternalOperationScopes names the scopes each operation of the external API needs, as the",
		"contract declares them (ADR 0061): an API token must carry all of them. An operation with no",
		"entry is refused to every token; an entry with no scopes is open to any valid token.",
	},
	Var:  "ExternalOperationScopes",
	Type: "map[externalapi.OperationName][]externalapi.ApiTokenScope",
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
		return fmt.Sprintf("externalapi.%sOperation: %s", op.Name(), names("externalapi.ApiTokenScope", scopes)), nil
	})(doc)
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
