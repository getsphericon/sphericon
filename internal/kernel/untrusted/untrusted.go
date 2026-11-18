// Package untrusted wraps, in a JSON value shaped by the OpenAPI contract, every field whose
// schema the contract marks `x-untrusted` (ADR 0016). Such a value originates outside the
// workspace, so it reaches a model as {"untrusted_data": <original value>}: data that is never an
// instruction. Everything that hands contract resources to a model wraps them here, so a field is
// wrapped by the same marks wherever it goes.
package untrusted

import (
	"bytes"
	"encoding/json"

	"github.com/getkin/kin-openapi/openapi3"
)

// extension marks, in the OpenAPI contract, a field whose value originates outside the
// workspace.
const extension = "x-untrusted"

// Key is the single key of the marker object that replaces an untrusted value:
// {"untrusted_data": <original value>}.
const Key = "untrusted_data"

// WrapJSON is Wrap over JSON text: it decodes the first JSON value of text, wraps it by ref and
// encodes it again, keeping numbers as written and markup unescaped, without a trailing newline.
// Text that does not start with a JSON value is an error; anything after the first value is
// dropped.
func WrapJSON(text []byte, ref *openapi3.SchemaRef) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	wrapped := Wrap(v, ref)
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(wrapped); err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(out.Bytes())
	return trimmed, nil
}

// Wrap returns v, a decoded JSON value, with every value whose schema in ref carries the
// `x-untrusted` mark replaced by its marker object. It walks objects (their properties and
// additional properties), arrays and the allOf, oneOf and anyOf variants, rewriting maps and
// slices in place. Null stays null: there is no text to be mistaken for an instruction.
func Wrap(v any, ref *openapi3.SchemaRef) any {
	if v == nil || ref == nil || ref.Value == nil {
		return v
	}
	schema := ref.Value
	if marked, _ := schema.Extensions[extension].(bool); marked {
		return map[string]any{Key: v}
	}
	for _, variants := range []openapi3.SchemaRefs{schema.AllOf, schema.OneOf, schema.AnyOf} {
		for _, variant := range variants {
			v = Wrap(v, variant)
		}
	}
	switch n := v.(type) {
	case map[string]any:
		extra := schema.AdditionalProperties.Schema
		for k, val := range n {
			if p, ok := schema.Properties[k]; ok {
				n[k] = Wrap(val, p)
			} else if extra != nil {
				n[k] = Wrap(val, extra)
			}
		}
	case []any:
		for i, val := range n {
			n[i] = Wrap(val, schema.Items)
		}
	}
	return v
}
