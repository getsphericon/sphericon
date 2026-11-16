package boundgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	schemaRef      = "#/components/schemas/"
	parameterRef   = "#/components/parameters/"
	requestBodyRef = "#/components/requestBodies/"
)

// walker collects the bounds of what a client sends in one document: the request bodies and
// parameters of its operations, and every schema they reach. A component is walked once, at its
// own place, whatever refers to it.
type walker struct {
	doc   document
	seen  map[string]bool
	found map[place]map[string]string
}

// walk returns the bounds of what a client sends in doc, by place: the bound keywords each place
// declares, as Go literals.
func walk(doc document) (map[place]map[string]string, error) {
	w := &walker{doc: doc, seen: map[string]bool{}, found: map[place]map[string]string{}}
	for path, item := range doc.Paths {
		var shared []json.RawMessage
		if raw, ok := item["parameters"]; ok {
			if err := json.Unmarshal(raw, &shared); err != nil {
				return nil, fmt.Errorf("%s parameters: %w", path, err)
			}
		}
		for method, raw := range item {
			if !isMethod(method) {
				continue
			}
			if err := w.operation(path, method, raw, shared); err != nil {
				return nil, err
			}
		}
	}
	return w.found, nil
}

// isMethod reports whether a key of a path item is an operation.
func isMethod(key string) bool {
	return slices.Contains([]string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}, key)
}

func (w *walker) operation(path, method string, raw json.RawMessage, shared []json.RawMessage) error {
	var op struct {
		OperationID string            `json:"operationId"`
		Parameters  []json.RawMessage `json:"parameters"`
		RequestBody json.RawMessage   `json:"requestBody"`
	}
	if err := json.Unmarshal(raw, &op); err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	id := op.OperationID
	if id == "" {
		id = strings.ToUpper(method) + " " + path
	}
	for _, param := range slices.Concat(shared, op.Parameters) {
		if err := w.parameter(id, param); err != nil {
			return err
		}
	}
	if op.RequestBody == nil {
		return nil
	}
	return w.requestBody(id, op.RequestBody)
}

// parameter walks a parameter: a component one at its own place, an inline one at the operation's.
func (w *walker) parameter(operationID string, raw json.RawMessage) error {
	var param struct {
		Ref     string                     `json:"$ref"`
		Name    string                     `json:"name"`
		Schema  json.RawMessage            `json:"schema"`
		Content map[string]json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &param); err != nil {
		return err
	}
	at := place{operation: operationID, parameter: param.Name}
	if param.Ref != "" {
		name, ok := strings.CutPrefix(param.Ref, parameterRef)
		if !ok {
			return fmt.Errorf("a parameter refers to %s", param.Ref)
		}
		if w.seen[param.Ref] {
			return nil
		}
		w.seen[param.Ref] = true
		component, ok := w.doc.Components.Parameters[name]
		if !ok {
			return fmt.Errorf("a parameter refers to the missing %s", param.Ref)
		}
		return w.parameterAt(place{parameter: name}, component)
	}
	return w.parameterAt(at, raw)
}

func (w *walker) parameterAt(at place, raw json.RawMessage) error {
	var param struct {
		Schema  json.RawMessage            `json:"schema"`
		Content map[string]json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &param); err != nil {
		return err
	}
	if param.Schema != nil {
		if err := w.schema(at, param.Schema); err != nil {
			return err
		}
	}
	return w.content(at, param.Content)
}

// requestBody walks the schemas of a request body, a component one included.
func (w *walker) requestBody(operationID string, raw json.RawMessage) error {
	var body struct {
		Ref     string                     `json:"$ref"`
		Content map[string]json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if body.Ref != "" {
		name, ok := strings.CutPrefix(body.Ref, requestBodyRef)
		if !ok {
			return fmt.Errorf("a request body refers to %s", body.Ref)
		}
		component, ok := w.doc.Components.RequestBodies[name]
		if !ok {
			return fmt.Errorf("a request body refers to the missing %s", body.Ref)
		}
		return w.requestBody(operationID, component)
	}
	return w.content(place{operation: operationID}, body.Content)
}

// content walks the schema of each media type.
func (w *walker) content(at place, content map[string]json.RawMessage) error {
	for _, raw := range content {
		var media struct {
			Schema json.RawMessage `json:"schema"`
		}
		if err := json.Unmarshal(raw, &media); err != nil {
			return err
		}
		if media.Schema == nil {
			continue
		}
		if err := w.schema(at, media.Schema); err != nil {
			return err
		}
	}
	return nil
}

// schema records the bounds a schema declares at its place and walks what it reaches: a component
// it refers to at the component's place, its properties, items and map values below it, and the
// members of a composition at its own place.
func (w *walker) schema(at place, raw json.RawMessage) error {
	// A boolean schema (`additionalProperties: false`) bounds nothing.
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	var s map[string]json.RawMessage
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if ref, ok := s["$ref"]; ok {
		if err := w.ref(ref); err != nil {
			return err
		}
	}
	values, err := literals(s, boundKinds())
	if err != nil {
		return fmt.Errorf("%s %w", at, err)
	}
	if len(values) > 0 {
		w.found[at] = values
	}
	if raw, ok := s["properties"]; ok {
		var props map[string]json.RawMessage
		if err := json.Unmarshal(raw, &props); err != nil {
			return err
		}
		for name, prop := range props {
			if err := w.schema(at.below(name), prop); err != nil {
				return err
			}
		}
	}
	for key, suffix := range map[string]string{"items": "[]", "additionalProperties": "{}"} {
		if raw, ok := s[key]; ok {
			if err := w.schema(at.below(suffix), raw); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		raw, ok := s[key]
		if !ok {
			continue
		}
		var members []json.RawMessage
		if err := json.Unmarshal(raw, &members); err != nil {
			return err
		}
		for _, m := range members {
			if err := w.schema(at, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// ref walks the component schema a reference names, once.
func (w *walker) ref(raw json.RawMessage) error {
	var ref string
	if err := json.Unmarshal(raw, &ref); err != nil {
		return err
	}
	name, ok := strings.CutPrefix(ref, schemaRef)
	if !ok {
		return fmt.Errorf("a schema refers to %s", ref)
	}
	if w.seen[ref] {
		return nil
	}
	w.seen[ref] = true
	component, ok := w.doc.Components.Schemas[name]
	if !ok {
		return fmt.Errorf("a schema refers to the missing %s", ref)
	}
	return w.schema(place{schema: name}, component)
}

// below is the place of a property (or of `[]` items, `{}` map values) of the schema at p.
func (p place) below(name string) place {
	switch {
	case p.property == "":
		p.property = name
	case strings.HasPrefix(name, "[") || strings.HasPrefix(name, "{"):
		p.property += name
	default:
		p.property += "." + name
	}
	return p
}

// boundKinds are the kinds a client's value is bounded by.
func boundKinds() []struct{ keyword, suffix string } {
	var out []struct{ keyword, suffix string }
	for _, k := range kinds {
		if slices.Contains(keywords, k.keyword) {
			out = append(out, k)
		}
	}
	return out
}
