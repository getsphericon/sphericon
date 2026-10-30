package scopegen

import "fmt"

// scopesExtension is where an operation of /service declares the scopes it needs.
const scopesExtension = "x-required-scopes"

// ServiceScopes is the Service operation-to-scopes map (ADR 0026): every operation declares
// `x-required-scopes`, an empty list being one any valid credential may call.
var ServiceScopes = Table{
	Spec:    "openapi/service.openapi.json",
	Out:     "ee/staff/service_scopes_gen.go",
	Package: "staff",
	Imports: []string{`serviceapi "github.com/getsphericon/sphericon/gen/service"`},
	Doc: []string{
		"ServiceOperationScopes names the scopes each operation of the service endpoints needs,",
		"as the contract declares them: a credential must carry all of them. An operation with no",
		"entry is refused to every credential; an entry with no scopes is open to any valid credential.",
	},
	Var:  "ServiceOperationScopes",
	Type: "map[serviceapi.OperationName][]serviceapi.ServiceScope",
	Rows: PerOperation(func(op Operation) (string, error) {
		scopes, declared, err := op.Strings(scopesExtension)
		if err != nil {
			return "", err
		}
		if !declared {
			return "", fmt.Errorf("%s declares no %s: declare its scopes in TypeSpec, an empty list for none", op, scopesExtension)
		}
		return fmt.Sprintf("serviceapi.%sOperation: %s", op.Name(), names("serviceapi.ServiceScope", scopes)), nil
	}),
}
