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

// Permissions is the Permission vocabulary in accounts (ADR 0061): the type a Role's table and
// every generated access table name, from the contract's Permission enum.
var Permissions = Table{
	Spec:    "openapi/site.openapi.json",
	Out:     "internal/accounts/permissions_gen.go",
	Package: "accounts",
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
	Out:     "internal/apitokens/scopes_gen.go",
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
