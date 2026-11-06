package scopegen

import (
	"fmt"
	"strings"
)

// Enum is the Decls of a file that gives a string enum of the contract a Go type of its own in a
// core package, so the package reads the contract's vocabulary without importing a surface's
// generated code (ADR 0061).
type Enum struct {
	// Schema is the component schema of the enum.
	Schema string
	// Doc is the comment over the type, one line per element.
	Doc []string
	// Type is the Go type; each value is the constant Prefix+valueName, in the contract's order.
	Type, Prefix string
	// List, when set, also declares a slice of every value under that name, commented by ListDoc.
	List    string
	ListDoc []string
}

// Decls renders the type, its constants and the list.
func (e Enum) Decls(doc Document) (string, error) {
	schemas, err := enumSchemas(doc)
	if err != nil {
		return "", err
	}
	values := schemas[e.Schema].values()
	if len(values) == 0 {
		return "", fmt.Errorf("the document has no %s enum", e.Schema)
	}
	var b strings.Builder
	writeDoc(&b, e.Doc)
	fmt.Fprintf(&b, "type %s string\n\n// The values of the contract's %s enum, in its order.\nconst (\n", e.Type, e.Schema)
	for _, v := range values {
		fmt.Fprintf(&b, "\t%s%s %s = %q\n", e.Prefix, valueName(v), e.Type, v)
	}
	b.WriteString(")\n")
	if e.List != "" {
		b.WriteString("\n")
		writeDoc(&b, e.ListDoc)
		fmt.Fprintf(&b, "var %s = []%s{\n", e.List, e.Type)
		for _, v := range values {
			fmt.Fprintf(&b, "\t%s%s,\n", e.Prefix, valueName(v))
		}
		b.WriteString("}\n")
	}
	return b.String(), nil
}

// Permissions is the Permission vocabulary in memberships (ADR 0061): the type a Role's table and
// every generated access table name, from the contract's Permission enum.
var Permissions = Table{
	Spec:    "openapi/site.openapi.json",
	Out:     "internal/domain/identity/memberships/permissions_gen.go",
	Package: "memberships",
	Decls: Enum{
		Schema: "Permission",
		Doc: []string{
			"Permission is a named action in a Workspace that a Role allows (GLOSSARY \"Permission\",",
			"ADR 0061), written `resource:verb`: a value of the contract's Permission enum.",
		},
		Type:   "Permission",
		Prefix: "Perm",
	}.Decls,
}

// TokenScopes is the API token scope vocabulary in apitokens: what a token may be minted with and
// the type the external API's scope table names, from the contract's ApiTokenScope enum.
var TokenScopes = Table{
	Spec:    "openapi/external.openapi.json",
	Out:     "internal/domain/identity/apitokens/scopes_gen.go",
	Package: "apitokens",
	Decls: Enum{
		Schema: "ApiTokenScope",
		Doc: []string{
			"Scope is what an API token may do in the external API: a value of the contract's",
			"ApiTokenScope enum, written `resource:verb`.",
		},
		Type:    "Scope",
		Prefix:  "Scope",
		List:    "Scopes",
		ListDoc: []string{"Scopes is the whole vocabulary: a token is minted with scopes from it only."},
	}.Decls,
}

// CredentialScopes is the Service credential scope vocabulary in the Enterprise module
// servicecredentials (ADR 0026): what a credential may be issued with, from the service
// contract's ServiceScope enum, so the module never imports the service surface's generated code.
var CredentialScopes = Table{
	Spec:    "openapi/service.openapi.json",
	Out:     "ee/domain/identity/servicecredentials/scopes_gen.go",
	Package: "servicecredentials",
	Decls: Enum{
		Schema: "ServiceScope",
		Doc: []string{
			"Scope is what a Service credential may do on the service endpoints: a value of the",
			"contract's ServiceScope enum, written `resource:verb`.",
		},
		Type:    "Scope",
		Prefix:  "Scope",
		List:    "Scopes",
		ListDoc: []string{"Scopes is the whole vocabulary: a credential is issued with scopes from it only."},
	}.Decls,
}

// UserScopes is the vocabulary of the scopes a User may give a credential of their own (ADR 0061),
// in the kernel package tokenscope: what an OAuth client is granted by default (GrantableScope),
// what needs the User's explicit opt-in (SendScope, SetupScope), and their union a Personal token
// is minted from (PersonalScope). A core package reads it from there and never from the /site
// surface's generated code.
var UserScopes = Table{
	Spec:    "openapi/site.openapi.json",
	Out:     "internal/kernel/tokenscope/user_scopes_gen.go",
	Package: "tokenscope",
	Decls: Enums(
		Enum{
			Schema:  "GrantableScope",
			Doc:     []string{"GrantableScope is a scope an OAuth client is granted without any extra opt-in."},
			Type:    "GrantableScope",
			Prefix:  "GrantableScope",
			List:    "GrantableScopes",
			ListDoc: []string{"GrantableScopes is every GrantableScope, in the contract's order."},
		},
		Enum{
			Schema:  "SendScope",
			Doc:     []string{"SendScope is a send-class scope (ADR 0016): granted only on the User's explicit opt-in."},
			Type:    "SendScope",
			Prefix:  "SendScope",
			List:    "SendScopes",
			ListDoc: []string{"SendScopes is every SendScope, in the contract's order."},
		},
		Enum{
			Schema: "SetupScope",
			Doc: []string{
				"SetupScope is a scope that changes how the Workspace sends (ADR 0016): granted only on an",
				"opt-in of its own.",
			},
			Type:    "SetupScope",
			Prefix:  "SetupScope",
			List:    "SetupScopes",
			ListDoc: []string{"SetupScopes is every SetupScope, in the contract's order."},
		},
		Enum{
			Schema: "PersonalScope",
			Doc: []string{
				"PersonalScope is a scope a User may give a credential of their own: a Connection by",
				"consenting, a Personal token by minting it (ADR 0061).",
			},
			Type:    "PersonalScope",
			Prefix:  "PersonalScope",
			List:    "PersonalScopes",
			ListDoc: []string{"PersonalScopes is every PersonalScope, in the contract's order."},
		},
	),
}

// Enums is the Decls of a file that declares several enums of one document, in the order given.
func Enums(enums ...Enum) func(Document) (string, error) {
	return func(doc Document) (string, error) {
		decls := make([]string, 0, len(enums))
		for _, e := range enums {
			d, err := e.Decls(doc)
			if err != nil {
				return "", err
			}
			decls = append(decls, d)
		}
		return strings.Join(decls, "\n"), nil
	}
}
