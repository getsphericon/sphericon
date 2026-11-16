// Package boundgen writes the value bounds of the TypeSpec contract into Go constants of the kernel
// package bounds (ADR 0107): the contract stays the one place a bound is written, and the domain
// module that owns the field checks every write against the constant, whichever driver made it.
// Bounds lists the bounds the core checks, each with the schema and property the contract declares
// it on and its owner; a bound the contract does not declare fails generation. Every bound of every
// document on what a client sends is accounted for: an entry of Bounds generates it, or one of
// TransportOnly says why only the transport checks it, and a bound neither names fails generation.
package boundgen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Out is the file cmd/boundgen writes, relative to the repository root.
const Out = "internal/kernel/bounds/bounds_gen.go"

// Bound is one bounded value of the contract: the length, range, item count, pattern or default the
// schema (or the property of the schema, or the shared parameter) declares, written as constants
// named Name plus the kind of bound (`MinLength`, `MaxLength`, `Min`, `Max`, `MinItems`,
// `MaxItems`, `Pattern`, `Default`). Entries that share a Name are one bound the contract declares
// in several places (a field of the site and of the external input): the first carries Doc and
// Owners, the others name only their place, and every place declares the same values. A place is
// named without its document: the entry accounts for it in every document that declares it, and
// each must declare the values of the entry's Spec.
type Bound struct {
	// Name is the stem of the constants; Doc continues the sentence "<Name> ..." over them.
	Name, Doc string
	// Spec is the OpenAPI document, relative to the repository root; Schema the component schema
	// and Property its property, or empty for a scalar; Parameter, in place of Schema, a component
	// parameter (a query parameter every list shares).
	Spec, Schema, Property, Parameter string
	// Owners are what checks the bound: an ent entity, whose owner in entrywrite's map writes the
	// field, or the package path of the owner when no column holds the value (an Action's header
	// count, the kernel's name rule).
	Owners []string
}

// kind is a JSON Schema keyword a bound is read from and the suffix of its constant.
type kind struct{ keyword, suffix string }

// boundKinds are the kinds that bound a client's value, in the order they are written.
var boundKinds = []kind{
	{"minLength", "MinLength"},
	{"maxLength", "MaxLength"},
	{"minimum", "Min"},
	{"maximum", "Max"},
	{"minItems", "MinItems"},
	{"maxItems", "MaxItems"},
	{"pattern", "Pattern"},
}

// kinds are every kind a constant is written for: the bounds, then the default, which bounds
// nothing.
var kinds = append(slices.Clone(boundKinds), kind{"default", "Default"})

// TransportBound is a bound of what a client sends that only the transport checks, because it
// never reaches a write: a parameter of a read, the form of an identifier ogen parses before the
// domain sees it, a value the app sends and never receives. It names its place as a Bound does,
// or an inline parameter of an operation by Operation and Parameter.
type TransportBound struct {
	Spec, Schema, Property, Parameter, Operation string
	// Doc says why no write meets the bound.
	Doc string
}

// ReadSpecs reads every OpenAPI document of the repository at root, by its path relative to root.
func ReadSpecs(root string) (map[string][]byte, error) {
	paths, err := filepath.Glob(filepath.Join(root, "openapi", "*.openapi.json"))
	if err != nil {
		return nil, err
	}
	specs := map[string][]byte{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		specs[filepath.ToSlash(filepath.Join("openapi", filepath.Base(path)))] = raw
	}
	return specs, nil
}

// TSTarget is a TypeScript package that owns bounds of its own (a client that cannot import the
// generated zod, such as the tracker under its size budget): the bounds whose Owners name Owner are
// written to Out as exported constants.
type TSTarget struct {
	Owner, Out string
}

// TSTargets are the TypeScript packages that read bounds.
var TSTargets = []TSTarget{
	{Owner: "packages/analytics/src", Out: "packages/analytics/src/bounds.gen.ts"},
}

// Write reads the documents and writes Out and the file of every TSTarget, from the repository
// root.
func Write() error {
	specs, err := ReadSpecs(".")
	if err != nil {
		return err
	}
	src, err := Generate(specs, Bounds, TransportOnly)
	if err != nil {
		return err
	}
	if err := os.WriteFile(Out, src, 0o600); err != nil {
		return err
	}
	for _, t := range TSTargets {
		ts, err := GenerateTS(specs, Bounds, t.Owner)
		if err != nil {
			return err
		}
		if err := os.WriteFile(t.Out, ts, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// GenerateTS renders the TypeScript constants of the bounds owner owns.
func GenerateTS(specs map[string][]byte, bounds []Bound, owner string) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("// Code generated by cmd/boundgen. DO NOT EDIT.\n")
	for _, b := range bounds {
		if !slices.Contains(b.Owners, owner) {
			continue
		}
		doc, err := parse(specs[b.Spec])
		if err != nil {
			return nil, fmt.Errorf("boundgen: %s: %w", b.Spec, err)
		}
		values, err := read(doc, b.place())
		if err != nil {
			return nil, fmt.Errorf("boundgen: %s: %w", b.Spec, err)
		}
		fmt.Fprintf(&out, "\n// %s %s\n// From %s.\n", b.Name, b.Doc, b.where())
		for _, k := range kinds {
			if v, ok := values[k.keyword]; ok {
				fmt.Fprintf(&out, "export const %s%s = %s\n", b.Name, k.suffix, tsLiteral(k.keyword, v))
			}
		}
	}
	return out.Bytes(), nil
}

// tsLiteral is a Go literal of literal as TypeScript: a pattern becomes a string.
func tsLiteral(keyword, v string) string {
	if keyword != "pattern" {
		return v
	}
	if s, err := strconv.Unquote(v); err == nil {
		return strconv.Quote(s)
	}
	return strconv.Quote(strings.Trim(v, "`"))
}

// document is what boundgen reads of an OpenAPI document: its operations and its components.
type document struct {
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas       map[string]json.RawMessage `json:"schemas"`
		Parameters    map[string]json.RawMessage `json:"parameters"`
		RequestBodies map[string]json.RawMessage `json:"requestBodies"`
	} `json:"components"`
}

// parse reads an OpenAPI document.
func parse(raw []byte) (document, error) {
	if raw == nil {
		return document{}, errors.New("the document is not read")
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return document{}, err
	}
	return doc, nil
}

// Generate renders the bounds package for the documents, by path. It fails unless every bound of
// what a client sends, in every document, is accounted for by an entry of bounds or of transport,
// and names every one left over.
func Generate(specs map[string][]byte, bounds []Bound, transport []TransportBound) ([]byte, error) {
	docs := map[string]document{}
	for path, raw := range specs {
		doc, err := parse(raw)
		if err != nil {
			return nil, fmt.Errorf("boundgen: %s: %w", path, err)
		}
		docs[path] = doc
	}
	stems, values, err := readBounds(docs, bounds)
	if err != nil {
		return nil, err
	}
	if err := account(docs, bounds, values, transport); err != nil {
		return nil, err
	}

	var consts, owners strings.Builder
	for _, b := range stems {
		fmt.Fprintf(&consts, "\n// %s %s\n// From %s.\nconst (\n", b.Name, b.Doc, strings.Join(placesOf(bounds, b.Name), ", "))
		for _, k := range kinds {
			v, ok := values[b.Name][k.keyword]
			if !ok {
				continue
			}
			fmt.Fprintf(&consts, "\t%s%s = %s\n", b.Name, k.suffix, v)
			quoted := make([]string, len(b.Owners))
			for i, o := range b.Owners {
				quoted[i] = strconv.Quote(o)
			}
			fmt.Fprintf(&owners, "\t%q: {%s},\n", b.Name+k.suffix, strings.Join(quoted, ", "))
		}
		consts.WriteString(")\n")
	}

	var out bytes.Buffer
	out.WriteString("// Code generated by cmd/boundgen. DO NOT EDIT.\n\npackage bounds\n")
	out.WriteString(consts.String())
	out.WriteString("\n// Owners names what checks each bound: an ent entity, whose owner writes the field, or the\n")
	out.WriteString("// package that owns it when no column holds the value. A test refuses a bound its owner does not use.\n")
	out.WriteString("var Owners = map[string][]string{\n")
	out.WriteString(owners.String())
	out.WriteString("}\n")
	return format.Source(out.Bytes())
}

// readBounds reads the values of every bound, by Name, and returns the first entry of each Name
// in the order of the list: the one that carries its Doc and Owners.
func readBounds(docs map[string]document, bounds []Bound) ([]Bound, map[string]map[string]string, error) {
	var stems []Bound
	values := map[string]map[string]string{}
	for _, b := range bounds {
		v, err := read(docs[b.Spec], b.place())
		if err != nil {
			return nil, nil, fmt.Errorf("boundgen: %s: %w", b.Spec, err)
		}
		first, repeated := values[b.Name]
		if !repeated {
			if len(b.Owners) == 0 {
				return nil, nil, fmt.Errorf("boundgen: %s names no owner", b.Name)
			}
			values[b.Name] = v
			stems = append(stems, b)
			continue
		}
		if b.Doc != "" || len(b.Owners) > 0 {
			return nil, nil, fmt.Errorf("boundgen: %s repeats the bound %s: it names only its place, the first carries Doc and Owners", b.where(), b.Name)
		}
		if !maps.Equal(first, v) {
			return nil, nil, fmt.Errorf("boundgen: %s declares %v, unlike the first place of %s, %v", b.where(), v, b.Name, first)
		}
	}
	return stems, values, nil
}

// account fails unless every bound of what a client sends, in every document, is a place of a
// bound with the same values or of a transport entry, and every transport entry names a bound its
// document declares.
func account(docs map[string]document, bounds []Bound, values map[string]map[string]string, transport []TransportBound) error {
	generated := map[place]Bound{}
	for _, b := range bounds {
		if _, ok := generated[b.place()]; ok {
			return fmt.Errorf("boundgen: %s is named by two entries of Bounds", b.where())
		}
		generated[b.place()] = b
	}
	checked := map[place]bool{}
	var problems []string
	for _, t := range transport {
		p := t.place()
		if _, ok := generated[p]; ok {
			problems = append(problems, where(t.Spec, p)+" is named by both Bounds and TransportOnly")
		}
		if checked[p] {
			problems = append(problems, where(t.Spec, p)+" is named twice by TransportOnly")
		}
		checked[p] = true
		if t.Doc == "" {
			problems = append(problems, fmt.Sprintf("TransportOnly %s says no reason", where(t.Spec, p)))
		}
		values, err := read(docs[t.Spec], p)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("TransportOnly %s: %v", where(t.Spec, p), err))
		case !bounded(values):
			problems = append(problems, fmt.Sprintf("TransportOnly %s declares only a default, which bounds nothing", where(t.Spec, p)))
		}
	}
	for _, path := range slices.Sorted(maps.Keys(docs)) {
		found, err := walk(docs[path])
		if err != nil {
			return fmt.Errorf("boundgen: %s: %w", path, err)
		}
		for _, p := range slices.SortedFunc(maps.Keys(found), place.compare) {
			if b, ok := generated[p]; ok {
				if !matches(values[b.Name], found[p]) {
					problems = append(problems, fmt.Sprintf("%s declares %v, unlike %s, %v", where(path, p), found[p], b.where(), values[b.Name]))
				}
				continue
			}
			if checked[p] {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s declares %s and no entry of Bounds or TransportOnly accounts for it",
				where(path, p), strings.Join(slices.Sorted(maps.Keys(found[p])), ", ")))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("boundgen: a bound of the contract is not accounted for:\n\t%s", strings.Join(problems, "\n\t"))
	}
	return nil
}

// bounded reports whether values hold a bound keyword, not only a default.
func bounded(values map[string]string) bool {
	return slices.ContainsFunc(boundKinds, func(k kind) bool { _, ok := values[k.keyword]; return ok })
}

// matches reports whether a place declares the bound keywords of a bound's values; the default is
// not a bound.
func matches(values, found map[string]string) bool {
	for _, k := range boundKinds {
		if values[k.keyword] != found[k.keyword] {
			return false
		}
	}
	return true
}

// place is where a document declares a bound, named without the document: a component schema
// (Schema), a property of one (Schema.Property, or a deeper path for an inline schema: `[]` the
// items of a list, `{}` the values of a map), a component parameter, or a parameter or the request
// body an operation declares inline.
type place struct {
	schema, property, parameter, operation string
}

func (p place) String() string {
	var base string
	switch {
	case p.operation != "" && p.parameter != "":
		base = "operation " + p.operation + " parameter " + p.parameter
	case p.operation != "":
		base = "operation " + p.operation + " body"
	case p.parameter != "":
		base = "parameter " + p.parameter
	default:
		base = p.schema
	}
	if p.property == "" {
		return base
	}
	return base + "." + p.property
}

func (p place) compare(q place) int { return strings.Compare(p.String(), q.String()) }

func (b Bound) place() place {
	return place{schema: b.Schema, property: b.Property, parameter: b.Parameter}
}

func (t TransportBound) place() place {
	return place{schema: t.Schema, property: t.Property, parameter: t.Parameter, operation: t.Operation}
}

// where names the bound's place in the contract.
func (b Bound) where() string { return where(b.Spec, b.place()) }

// where names a place of a document.
func where(spec string, p place) string {
	return strings.TrimSuffix(filepath.Base(spec), ".openapi.json") + " " + p.String()
}

// placesOf names every place of the bound name.
func placesOf(bounds []Bound, name string) []string {
	var out []string
	for _, b := range bounds {
		if b.Name == name {
			out = append(out, b.where())
		}
	}
	return out
}

// schemaOf is the schema of a place: the component schema, or the schema of a parameter.
func schemaOf(doc document, p place) (json.RawMessage, error) {
	switch {
	case p.operation != "":
		return inlineParameter(doc, p.operation, p.parameter)
	case p.parameter != "":
		raw, ok := doc.Components.Parameters[p.parameter]
		if !ok {
			return nil, fmt.Errorf("the document has no parameter %s", p.parameter)
		}
		var param struct {
			Schema json.RawMessage `json:"schema"`
		}
		if err := json.Unmarshal(raw, &param); err != nil {
			return nil, err
		}
		return param.Schema, nil
	}
	raw, ok := doc.Components.Schemas[p.schema]
	if !ok {
		return nil, fmt.Errorf("the document has no schema %s", p.schema)
	}
	return raw, nil
}

// inlineParameter is the schema of the parameter an operation declares inline.
func inlineParameter(doc document, operationID, name string) (json.RawMessage, error) {
	for _, item := range doc.Paths {
		for method, raw := range item {
			if !isMethod(method) {
				continue
			}
			var op struct {
				OperationID string `json:"operationId"`
				Parameters  []struct {
					Name   string          `json:"name"`
					Schema json.RawMessage `json:"schema"`
				} `json:"parameters"`
			}
			if err := json.Unmarshal(raw, &op); err != nil {
				return nil, err
			}
			if op.OperationID != operationID {
				continue
			}
			for _, param := range op.Parameters {
				if param.Name == name {
					return param.Schema, nil
				}
			}
			return nil, fmt.Errorf("the operation %s has no inline parameter %s", operationID, name)
		}
	}
	return nil, fmt.Errorf("the document has no operation %s", operationID)
}

// read returns the bound's keywords at a place as Go literals.
func read(doc document, p place) (map[string]string, error) {
	raw, err := schemaOf(doc, p)
	if err != nil {
		return nil, err
	}
	if p.property != "" {
		var obj struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
		var ok bool
		if raw, ok = obj.Properties[p.property]; !ok {
			return nil, fmt.Errorf("the document has no property %s.%s", p.schema, p.property)
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	values, err := literals(fields, kinds)
	if err != nil {
		return nil, fmt.Errorf("%s %w", p, err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s declares no bound", p)
	}
	return values, nil
}

// literals are the Go literals of the keywords of kinds a schema declares.
func literals(fields map[string]json.RawMessage, of []kind) (map[string]string, error) {
	values := map[string]string{}
	for _, k := range of {
		v, ok := fields[k.keyword]
		if !ok {
			continue
		}
		lit, err := literal(k.keyword, v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k.keyword, err)
		}
		values[k.keyword] = lit
	}
	return values, nil
}

// literal is the Go literal of a keyword's value: a raw string for a pattern, an integer otherwise.
func literal(keyword string, v json.RawMessage) (string, error) {
	if keyword == "pattern" {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return "", err
		}
		if strings.Contains(s, "`") {
			return strconv.Quote(s), nil
		}
		return "`" + s + "`", nil
	}
	var n json.Number
	if err := json.Unmarshal(v, &n); err != nil {
		return "", err
	}
	if _, err := n.Int64(); err != nil {
		return "", errors.New("not an integer")
	}
	return n.String(), nil
}
