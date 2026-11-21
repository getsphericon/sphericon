package scopegen

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Enum is the Decls of a file that declares a string enum of the contract in a core package, so the
// package reads the contract's vocabulary without importing a surface's generated code (ADR 0061).
type Enum struct {
	// Schema is the component schema of the enum.
	Schema string
	// Doc is the comment over the type, or over the constants when there is no type, one line per
	// element.
	Doc []string
	// Type is the Go type; each value is the constant Prefix+valueName, in the contract's order. With
	// no Type the constants are untyped, for a vocabulary whose values share their fields with values
	// outside it (a System event's action and a customer's own), and the list is a []string.
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
	listType := "string"
	if e.Type != "" {
		listType = e.Type
		fmt.Fprintf(&b, "type %s string\n\n", e.Type)
	} else if len(e.Doc) > 0 {
		b.WriteString("//\n")
	}
	fmt.Fprintf(&b, "// The values of the contract's %s enum, in its order.\nconst (\n", e.Schema)
	for _, v := range values {
		fmt.Fprintf(&b, "\t%s%s %s = %q\n", e.Prefix, valueName(v), e.Type, v)
	}
	b.WriteString(")\n")
	if e.List != "" {
		b.WriteString("\n")
		writeDoc(&b, e.ListDoc)
		fmt.Fprintf(&b, "var %s = []%s{\n", e.List, listType)
		for _, v := range values {
			fmt.Fprintf(&b, "\t%s%s,\n", e.Prefix, valueName(v))
		}
		b.WriteString("}\n")
	}
	return b.String(), nil
}

// SystemEventActions is the actions of the System events in the kernel package systemevent, from
// the contract's SystemEventAction enum: the bus types of the events recorded as System events and
// the actions their Event rows carry. They are untyped, as an Event's action is a string that may
// be a customer's own.
var SystemEventActions = Table{
	Spec:    siteSpec,
	Out:     "internal/kernel/systemevent/actions_gen.go",
	Package: "systemevent",
	Decls: Enum{
		Schema: "SystemEventAction",
		Doc: []string{
			"The actions of the System events: what sphericon itself records, never a customer, each also",
			"the bus type of the event that records it (TestEverySystemEventIsTheContracts).",
		},
		List:    "Actions",
		ListDoc: []string{"Actions is every System event action, in the contract's order."},
	}.Decls,
}

// ConditionCombinators is how a condition Group joins its entries, from the
// contract's ConditionCombinator enum.
var ConditionCombinators = Table{
	Spec:    siteSpec,
	Out:     "internal/kernel/conditiontree/combinator_gen.go",
	Package: "conditiontree",
	Decls: Enum{
		Schema:  "ConditionCombinator",
		Doc:     []string{"Combinator is how a Group joins its entries: And, every entry matches; Or, any entry does."},
		Type:    "Combinator",
		List:    "Combinators",
		ListDoc: []string{"Combinators is every Combinator, in the contract's order."},
	}.Decls,
}

// ConditionAttributes is the text attributes of a Contact an attribute leaf of a Condition tests,
// from the contract's ContactAttribute enum.
var ConditionAttributes = Table{
	Spec:    siteSpec,
	Out:     "internal/kernel/conditiontree/attribute_gen.go",
	Package: "conditiontree",
	Decls: Enum{
		Schema:  "ContactAttribute",
		Doc:     []string{"Attribute is a text attribute of a Contact that an attribute leaf tests."},
		Type:    "Attribute",
		Prefix:  "Attribute",
		List:    "Attributes",
		ListDoc: []string{"Attributes is every Attribute an attribute leaf may test, in the contract's order."},
	}.Decls,
}

// ConditionOperators is how each test and leaf of a Condition compares, from the contract's
// operator enums, so the module that decodes a Condition reads its operators from the contract.
var ConditionOperators = Table{
	Spec:    siteSpec,
	Out:     "internal/kernel/conditiontree/operators_gen.go",
	Package: "conditiontree",
	Decls: Enums(
		operatorEnum("TextOperator", "Text", "how a text test compares the value with its own"),
		operatorEnum("NumberOperator", "Number", "how a number test compares the value with its own"),
		operatorEnum("MomentOperator", "Moment", "which side of its moment a moment test asks for"),
		operatorEnum("PresenceOperator", "Presence", "whether a presence test asks for a value set or not"),
		operatorEnum("HasOperator", "", "whether a Tag, Telegram or SMS leaf asks the Contact to have it"),
		operatorEnum("EventCountOperator", "", "how an event leaf bounds the count of the Contact's Events"),
	),
}

// operatorEnum is the Enum of the contract's operator enum schema, typed by its name, with its list.
func operatorEnum(schema, prefix, what string) Enum {
	return Enum{
		Schema:  schema,
		Doc:     []string{schema + " is " + what + "."},
		Type:    schema,
		Prefix:  prefix,
		List:    schema + "s",
		ListDoc: []string{schema + "s is every " + schema + ", in the contract's order."},
	}
}

// ConditionKinds is the kinds of a Condition's entries, tests and sources, from the
// discriminators of the contract's unions, and which of them each place and each leaf admits: the
// union of the entries of a place, of the tests of a leaf, of the sources of a property test.
var ConditionKinds = Table{
	Spec:    siteSpec,
	Out:     "internal/kernel/conditiontree/kinds_gen.go",
	Package: "conditiontree",
	Decls: Discriminated(
		Discriminators{
			Doc:    []string{"Kind is the kind of an entry of a Condition's group: a nested group or a leaf."},
			Type:   "Kind",
			Prefix: "Kind",
			Unions: []Union{
				{Schema: "SegmentConditionNode", List: "SegmentKinds", Doc: []string{"SegmentKinds is every Kind a Segment's Condition admits."}},
				{Schema: "RuleConditionNode", List: "RuleKinds", Doc: []string{"RuleKinds is every Kind a Rule's Condition admits."}},
			},
		},
		Discriminators{
			Doc:    []string{"TestKind is the kind of a leaf's test: what it asks of the value the leaf reads."},
			Type:   "TestKind",
			Prefix: "Test",
			Unions: []Union{
				{Schema: "AttributeTest", List: "AttributeTests", Doc: []string{"AttributeTests is every TestKind an attribute leaf takes."}},
				{Schema: "BirthdayTest", List: "BirthdayTests", Doc: []string{"BirthdayTests is every TestKind a birthday leaf takes."}},
				{Schema: "CustomFieldTest", List: "CustomFieldTests", Doc: []string{
					"CustomFieldTests is every TestKind a Custom field leaf takes, whichever its field's type.",
				}},
				{Schema: "PropertyTest", List: "PropertyTests", Doc: []string{"PropertyTests is every TestKind a property leaf takes."}},
			},
		},
		Discriminators{
			Doc:    []string{"Source is whose properties a property leaf reads."},
			Type:   "Source",
			Prefix: "Source",
			Unions: []Union{
				{Schema: "RulePropertySource", List: "RuleSources", Doc: []string{"RuleSources is every Source a Rule's property leaf reads."}},
			},
		},
	),
}

// Discriminators is the Decls of the discriminator values of the contract's unions: a type, a
// constant per value of any of the unions (in the order the unions first name them) and, per union,
// the list of the values it admits, so a core package checks what a place admits by the contract.
type Discriminators struct {
	// Doc is the comment over the type, one line per element.
	Doc []string
	// Type is the Go type; each value is the constant Prefix+valueName.
	Type, Prefix string
	// Unions are the unions read, each listed under its List.
	Unions []Union
}

// Union is a discriminated union of the contract and the list of the values it admits.
type Union struct {
	// Schema is the component schema of the union, List the slice declared, Doc its comment.
	Schema, List string
	Doc          []string
}

// unionSchema is a component schema as a discriminated union: its members in order and the
// discriminator's value of each.
type unionSchema struct {
	OneOf []struct {
		Ref string `json:"$ref"`
	} `json:"oneOf"`
	Discriminator *struct {
		Mapping map[string]string `json:"mapping"`
	} `json:"discriminator"`
}

// values are the discriminator's values of the union's members, in the order of oneOf.
func (u unionSchema) values() ([]string, error) {
	if u.Discriminator == nil || len(u.OneOf) == 0 {
		return nil, nil
	}
	byRef := make(map[string]string, len(u.Discriminator.Mapping))
	for value, ref := range u.Discriminator.Mapping {
		byRef[ref] = value
	}
	out := make([]string, 0, len(u.OneOf))
	for _, member := range u.OneOf {
		value, ok := byRef[member.Ref]
		if !ok {
			return nil, fmt.Errorf("the member %s has no discriminator value", member.Ref)
		}
		out = append(out, value)
	}
	return out, nil
}

// Decls renders the type, its constants and a list per union.
func (d Discriminators) Decls(doc Document) (string, error) {
	var raw struct {
		Components struct {
			Schemas map[string]unionSchema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(doc.Raw, &raw); err != nil {
		return "", err
	}
	var all []string
	lists := make([][]string, len(d.Unions))
	for i, u := range d.Unions {
		values, err := raw.Components.Schemas[u.Schema].values()
		if err != nil {
			return "", fmt.Errorf("%s: %w", u.Schema, err)
		}
		if len(values) == 0 {
			return "", fmt.Errorf("the document has no %s discriminated union", u.Schema)
		}
		lists[i] = values
		for _, v := range values {
			if !slices.Contains(all, v) {
				all = append(all, v)
			}
		}
	}
	var b strings.Builder
	writeDoc(&b, d.Doc)
	fmt.Fprintf(&b, "type %s string\n\n", d.Type)
	b.WriteString("// The discriminator values of the contract's unions, in the order they first name them.\nconst (\n")
	for _, v := range all {
		fmt.Fprintf(&b, "\t%s%s %s = %q\n", d.Prefix, valueName(v), d.Type, v)
	}
	b.WriteString(")\n")
	for i, u := range d.Unions {
		b.WriteString("\n")
		writeDoc(&b, u.Doc)
		fmt.Fprintf(&b, "var %s = []%s{\n", u.List, d.Type)
		for _, v := range lists[i] {
			fmt.Fprintf(&b, "\t%s%s,\n", d.Prefix, valueName(v))
		}
		b.WriteString("}\n")
	}
	return b.String(), nil
}

// Discriminated is the Decls of a file that declares the discriminators of several sets of unions,
// in the order given.
func Discriminated(sets ...Discriminators) func(Document) (string, error) {
	return func(doc Document) (string, error) {
		decls := make([]string, 0, len(sets))
		for _, d := range sets {
			out, err := d.Decls(doc)
			if err != nil {
				return "", err
			}
			decls = append(decls, out)
		}
		return strings.Join(decls, "\n"), nil
	}
}

// Permissions is the Permission vocabulary in memberships (ADR 0061): the type a Role's table and
// every generated access table name, from the contract's Permission enum.
var Permissions = Table{
	Spec:    siteSpec,
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
	Spec:    externalSpec,
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
	Spec:    serviceSpec,
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
	Spec:    siteSpec,
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
