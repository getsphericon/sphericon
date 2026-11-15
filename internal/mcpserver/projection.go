package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	schemaRefPrefix = "#/components/schemas/"
	defsRefPrefix   = "#/$defs/"
	bodyArgument    = "body"
)

// parameter is an OpenAPI path/query/header parameter, exposed as a tool argument.
type parameter struct {
	name string
	in   string // path | query | header
}

// operation is one external-API operation projected as a tool.
type operation struct {
	tool       *mcp.Tool
	method     string
	path       string
	params     []parameter
	bodyFields map[string]bool     // top-level body properties exposed as arguments
	bodyWhole  bool                // the body is not an object: it is passed as the `body` argument
	response   *openapi3.SchemaRef // success schema, to mark untrusted fields in results
	hasBody    bool
	send       bool // send-class (x-mcp send): listed and callable only with the mcp:send scope
}

// projections holds the projection of each document New was given, by its bytes: the operations
// depend on the document alone and nothing changes them once built, so every server (an Instance,
// a test's) shares them and binds only its own handlers. A process sees one document.
var (
	projectionsMu sync.Mutex
	projections   = map[string]projection{}
)

type projection struct {
	ops []*operation
	err error
}

// projected is project of spec, computed once per document.
func projected(spec []byte) ([]*operation, error) {
	projectionsMu.Lock()
	defer projectionsMu.Unlock()
	p, ok := projections[string(spec)]
	if !ok {
		p.ops, p.err = project(spec)
		projections[string(spec)] = p
	}
	return p.ops, p.err
}

// project turns the OpenAPI document into one operation per non-hidden
// operation, in a stable order. Tool names come from `x-mcp.name` or, by
// default, the operation id in snake_case (`Contacts_list` -> `contacts_list`).
func project(spec []byte) ([]*operation, error) {
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		return nil, fmt.Errorf("parse OpenAPI document: %w", err)
	}
	schemas, err := componentSchemas(doc)
	if err != nil {
		return nil, err
	}

	var ops []*operation
	seen := map[string]string{}
	paths := doc.Paths.Map()
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			raw := paths[path].GetOperation(method)
			if raw == nil {
				continue
			}
			lower := strings.ToLower(method)
			ext, _ := raw.Extensions["x-mcp"].(map[string]any)
			if hidden, _ := ext["hidden"].(bool); hidden {
				continue
			}
			name, _ := ext["name"].(string)
			if name == "" {
				name = defaultToolName(raw.OperationID)
			}
			if name == "" {
				return nil, fmt.Errorf("%s %s: no operationId or x-mcp name", lower, path)
			}
			if prev, dup := seen[name]; dup {
				return nil, fmt.Errorf("tool name %q is used by both %s and %s %s", name, prev, lower, path)
			}
			seen[name] = lower + " " + path

			op, err := buildOperation(name, method, path, raw, schemas)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", lower, path, err)
			}
			op.send, _ = ext["send"].(bool)
			ops = append(ops, op)
		}
	}
	return ops, nil
}

// componentSchemas is the JSON of each component schema by its name, the source of the `$defs` of
// the input schemas.
func componentSchemas(doc *openapi3.T) (map[string]any, error) {
	schemas := map[string]any{}
	if doc.Components == nil {
		return schemas, nil
	}
	for name, ref := range doc.Components.Schemas {
		schema, err := schemaJSON(ref)
		if err != nil {
			return nil, fmt.Errorf("component schema %s: %w", name, err)
		}
		schemas[name] = schema
	}
	return schemas, nil
}

// arguments collects the properties of one tool's input schema: the operation's parameters and the
// fields of its body, which share one namespace.
type arguments struct {
	props    map[string]any
	required []string
}

func (a *arguments) add(argument string, schema map[string]any, isRequired bool) error {
	if _, dup := a.props[argument]; dup {
		return fmt.Errorf("argument %q is declared twice (parameter and body field)", argument)
	}
	a.props[argument] = schema
	if isRequired {
		a.required = append(a.required, argument)
	}
	return nil
}

// inputSchema is the JSON schema of the arguments, with the definitions its properties refer to.
func (a *arguments) inputSchema(schemas map[string]any) map[string]any {
	slices.Sort(a.required)
	input := map[string]any{"type": "object", "properties": a.props}
	if len(a.required) > 0 {
		input["required"] = a.required
	}
	input["additionalProperties"] = false
	if defs := collectDefs(input, schemas); len(defs) > 0 {
		input["$defs"] = defs
	}
	return input
}

func buildOperation(name, method, path string, raw *openapi3.Operation, schemas map[string]any) (*operation, error) {
	op := &operation{method: method, path: path, bodyFields: map[string]bool{}, response: responseSchema(raw)}
	args := &arguments{props: map[string]any{}}
	if err := addParameters(op, args, raw.Parameters); err != nil {
		return nil, err
	}
	if raw.RequestBody != nil {
		if err := addBody(op, args, raw.RequestBody.Value); err != nil {
			return nil, err
		}
	}

	desc := raw.Description
	if desc == "" {
		desc = raw.Summary
	}
	op.tool = &mcp.Tool{
		Name:        name,
		Description: desc,
		InputSchema: args.inputSchema(schemas),
		Annotations: annotationsFor(method),
	}
	return op, nil
}

// addParameters makes an argument of each parameter of the operation.
func addParameters(op *operation, args *arguments, params openapi3.Parameters) error {
	for _, ref := range params {
		param := ref.Value
		if param == nil {
			return fmt.Errorf("unresolved parameter %s", ref.Ref)
		}
		schema, err := copySchema(param.Schema)
		if err != nil {
			return fmt.Errorf("parameter %s: %w", param.Name, err)
		}
		if param.Description != "" {
			schema["description"] = param.Description
		}
		if err := args.add(param.Name, schema, param.Required || param.In == openapi3.ParameterInPath); err != nil {
			return err
		}
		op.params = append(op.params, parameter{name: param.Name, in: param.In})
	}
	return nil
}

// addBody makes an argument of each field of a JSON request body, or one argument for a body that
// is not an object.
func addBody(op *operation, args *arguments, body *openapi3.RequestBody) error {
	var media *openapi3.MediaType
	if body != nil {
		media = body.Content["application/json"]
	}
	if media == nil || media.Schema == nil {
		return errors.New("request body is not application/json")
	}
	op.hasBody = true
	resolved := media.Schema.Value
	if resolved == nil || resolved.Properties == nil {
		op.bodyWhole = true
		schema, err := copySchema(media.Schema)
		if err != nil {
			return fmt.Errorf("request body: %w", err)
		}
		return args.add(bodyArgument, schema, body.Required)
	}
	for _, field := range slices.Sorted(maps.Keys(resolved.Properties)) {
		schema, err := copySchema(resolved.Properties[field])
		if err != nil {
			return fmt.Errorf("body field %s: %w", field, err)
		}
		if err := args.add(field, schema, slices.Contains(resolved.Required, field)); err != nil {
			return err
		}
		op.bodyFields[field] = true
	}
	return nil
}

// annotationsFor derives the MCP behavior hints from the HTTP method: GET reads;
// PUT replaces and DELETE removes (both destructive and idempotent); POST only
// adds (neither). Every tool talks to this system only, so it is not open-world.
func annotationsFor(method string) *mcp.ToolAnnotations {
	closed := false
	a := &mcp.ToolAnnotations{OpenWorldHint: &closed}
	switch method {
	case http.MethodGet:
		a.ReadOnlyHint = true
	case http.MethodPut, http.MethodDelete, http.MethodPatch:
		destructive := true
		a.DestructiveHint = &destructive
		a.IdempotentHint = method != http.MethodPatch
	default:
		destructive := false
		a.DestructiveHint = &destructive
	}
	return a
}

// defaultToolName turns `Contacts_list` into `contacts_list` and
// `EventActions_list` into `event_actions_list`.
func defaultToolName(operationID string) string {
	var b strings.Builder
	for i, r := range operationID {
		if unicode.IsUpper(r) {
			if i > 0 && operationID[i-1] != '_' {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// schemaJSON is a schema as its JSON reads, a reference left as one.
func schemaJSON(ref *openapi3.SchemaRef) (map[string]any, error) {
	if ref == nil {
		return map[string]any{}, nil
	}
	data, err := json.Marshal(ref)
	if err != nil {
		return nil, fmt.Errorf("encode schema: %w", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	return schema, nil
}

// copySchema is the JSON of a schema for a tool's input, with component refs rewritten to local
// `$defs` and OpenAPI 3.0 `nullable` to the JSON Schema `type: [t, "null"]` form.
func copySchema(ref *openapi3.SchemaRef) (map[string]any, error) {
	schema, err := schemaJSON(ref)
	if err != nil {
		return nil, err
	}
	return rewriteSchema(schema), nil
}

func rewriteSchema(in map[string]any) map[string]any {
	out, _ := rewrite(in).(map[string]any)
	return out
}

func rewrite(v any) any {
	switch n := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(n))
		for k, val := range n {
			if k == "$ref" {
				if s, ok := val.(string); ok {
					out[k] = defsRefPrefix + strings.TrimPrefix(s, schemaRefPrefix)
					continue
				}
			}
			out[k] = rewrite(val)
		}
		if nullable, _ := out["nullable"].(bool); nullable {
			if t, ok := out["type"].(string); ok {
				out["type"] = []any{t, "null"}
			}
		}
		delete(out, "nullable")
		return out
	case []any:
		out := make([]any, len(n))
		for i, val := range n {
			out[i] = rewrite(val)
		}
		return out
	default:
		return v
	}
}

// collectDefs returns the component schemas transitively referenced by the tool's
// input schema, rewritten for `$defs`.
func collectDefs(input, schemas map[string]any) map[string]any {
	defs := map[string]any{}
	var visit func(v any)
	visit = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if ref, ok := n["$ref"].(string); ok && strings.HasPrefix(ref, defsRefPrefix) {
				name := strings.TrimPrefix(ref, defsRefPrefix)
				if _, done := defs[name]; !done {
					if src, ok := schemas[name].(map[string]any); ok {
						rewritten := rewriteSchema(src)
						defs[name] = rewritten
						visit(rewritten)
					}
				}
			}
			for _, val := range n {
				visit(val)
			}
		case []any:
			for _, val := range n {
				visit(val)
			}
		}
	}
	visit(input)
	return defs
}
