package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	schemaRefPrefix = "#/components/schemas/"
	paramRefPrefix  = "#/components/parameters/"
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
	bodyFields map[string]bool    // top-level body properties exposed as arguments
	bodyWhole  bool               // the body is not an object: it is passed as the `body` argument
	response   *untrustedResponse // success schema, to mark untrusted fields in results
	hasBody    bool
	send       bool // send-class (x-mcp send): listed and callable only with the mcp:send scope
}

// project turns the OpenAPI document into one operation per non-hidden
// operation, in a stable order. Tool names come from `x-mcp.name` or, by
// default, the operation id in snake_case (`Contacts_list` -> `contacts_list`).
func project(spec []byte) ([]*operation, error) {
	var doc map[string]any
	if err := json.Unmarshal(spec, &doc); err != nil {
		return nil, fmt.Errorf("parse OpenAPI document: %w", err)
	}
	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	parameters, _ := components["parameters"].(map[string]any)
	paths, _ := doc["paths"].(map[string]any)

	var ops []*operation
	seen := map[string]string{}
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		item, _ := paths[path].(map[string]any)
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			raw, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			ext, _ := raw["x-mcp"].(map[string]any)
			if hidden, _ := ext["hidden"].(bool); hidden {
				continue
			}
			opID, _ := raw["operationId"].(string)
			name, _ := ext["name"].(string)
			if name == "" {
				name = defaultToolName(opID)
			}
			if name == "" {
				return nil, fmt.Errorf("%s %s: no operationId or x-mcp name", method, path)
			}
			if prev, dup := seen[name]; dup {
				return nil, fmt.Errorf("tool name %q is used by both %s and %s %s", name, prev, method, path)
			}
			seen[name] = method + " " + path

			op, err := buildOperation(name, strings.ToUpper(method), path, raw, schemas, parameters)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", method, path, err)
			}
			op.send, _ = ext["send"].(bool)
			ops = append(ops, op)
		}
	}
	return ops, nil
}

func buildOperation(name, method, path string, raw, schemas, parameters map[string]any) (*operation, error) {
	op := &operation{method: method, path: path, bodyFields: map[string]bool{}, response: responseSchema(raw, schemas)}
	props := map[string]any{}
	var required []string
	add := func(argument string, schema map[string]any, isRequired bool) error {
		if _, dup := props[argument]; dup {
			return fmt.Errorf("argument %q is declared twice (parameter and body field)", argument)
		}
		props[argument] = schema
		if isRequired {
			required = append(required, argument)
		}
		return nil
	}

	rawParams, _ := raw["parameters"].([]any)
	for _, p := range rawParams {
		param, _ := p.(map[string]any)
		if ref, ok := param["$ref"].(string); ok {
			resolved, ok := parameters[strings.TrimPrefix(ref, paramRefPrefix)].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("unresolved parameter %s", ref)
			}
			param = resolved
		}
		pname, _ := param["name"].(string)
		in, _ := param["in"].(string)
		schema, _ := param["schema"].(map[string]any)
		schema = copySchema(schema)
		if desc, ok := param["description"].(string); ok {
			schema["description"] = desc
		}
		isRequired, _ := param["required"].(bool)
		if err := add(pname, schema, isRequired || in == "path"); err != nil {
			return nil, err
		}
		op.params = append(op.params, parameter{name: pname, in: in})
	}

	if body, ok := raw["requestBody"].(map[string]any); ok {
		content, _ := body["content"].(map[string]any)
		media, _ := content["application/json"].(map[string]any)
		schema, _ := media["schema"].(map[string]any)
		if schema == nil {
			return nil, errors.New("request body is not application/json")
		}
		op.hasBody = true
		resolved := schema
		if ref, ok := schema["$ref"].(string); ok {
			resolved, _ = schemas[strings.TrimPrefix(ref, schemaRefPrefix)].(map[string]any)
		}
		bodyProps, isObject := resolved["properties"].(map[string]any)
		if !isObject {
			op.bodyWhole = true
			if err := add(bodyArgument, copySchema(schema), isRequiredBody(body)); err != nil {
				return nil, err
			}
		} else {
			bodyRequired, _ := resolved["required"].([]any)
			requiredSet := map[string]bool{}
			for _, r := range bodyRequired {
				if s, ok := r.(string); ok {
					requiredSet[s] = true
				}
			}
			for _, field := range slices.Sorted(maps.Keys(bodyProps)) {
				fieldSchema, _ := bodyProps[field].(map[string]any)
				if err := add(field, copySchema(fieldSchema), requiredSet[field]); err != nil {
					return nil, err
				}
				op.bodyFields[field] = true
			}
		}
	}

	slices.Sort(required)
	input := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		input["required"] = required
	}
	input["additionalProperties"] = false
	if defs := collectDefs(input, schemas); len(defs) > 0 {
		input["$defs"] = defs
	}

	desc, _ := raw["description"].(string)
	if desc == "" {
		desc, _ = raw["summary"].(string)
	}
	op.tool = &mcp.Tool{
		Name:        name,
		Description: desc,
		InputSchema: input,
		Annotations: annotationsFor(method),
	}
	return op, nil
}

func isRequiredBody(body map[string]any) bool {
	r, _ := body["required"].(bool)
	return r
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

// copySchema deep-copies a schema, rewriting component refs to local `$defs` and
// OpenAPI 3.0 `nullable` to the JSON Schema `type: [t, "null"]` form.
func copySchema(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
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
						rewritten := copySchema(src)
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
