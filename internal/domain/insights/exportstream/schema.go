package exportstream

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"

	sphericon "github.com/getsphericon/sphericon"
)

// document is the embedded external OpenAPI document, loaded on first use.
var document = sync.OnceValues(func() (*openapi3.T, error) {
	return openapi3.NewLoader().LoadFromData(sphericon.ExternalOpenAPI)
})

// ValidateRow reports why one encoded row (a line of a page, without its newline) does
// not match the TypeSpec row schema named by schema (an openapi component such as
// EventsSchema), or nil when it does. It is the check that keeps a stream's row type and
// its published contract the same thing: beyond the schema's own types and required keys,
// a key the schema does not declare is a failure too, because the generated documentation
// would not describe it.
func ValidateRow(schema string, line []byte) error {
	doc, err := document()
	if err != nil {
		return fmt.Errorf("exportstream: load openapi/external.openapi.json: %w", err)
	}
	ref, ok := doc.Components.Schemas[schema]
	if !ok {
		return fmt.Errorf("exportstream: openapi/external.openapi.json has no %s schema", schema)
	}
	var value map[string]any
	if err := json.Unmarshal(line, &value); err != nil {
		return fmt.Errorf("exportstream: row is not a JSON object: %w", err)
	}
	if err := ref.Value.VisitJSON(value); err != nil {
		return fmt.Errorf("exportstream: row does not match %s: %w", schema, err)
	}
	for key := range value {
		if _, declared := ref.Value.Properties[key]; !declared {
			return fmt.Errorf("exportstream: row has key %q that %s does not declare", key, schema)
		}
	}
	for key := range ref.Value.Properties {
		if _, present := value[key]; !present && !slices.Contains(ref.Value.Required, key) {
			return fmt.Errorf("exportstream: row lacks optional key %q: a stream always sends every key", key)
		}
	}
	return nil
}
