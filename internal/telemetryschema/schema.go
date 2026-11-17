// Package telemetryschema checks a Telemetry report against the published
// OpenAPI contract (ADR 0032), so the documented payload is the real payload.
package telemetryschema

import (
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"

	sphericon "github.com/getsphericon/sphericon"
)

// reportSchema is the TelemetryReport component of the embedded OpenAPI file, closed: the
// published schema leaves a property it does not declare to the receiver, which skips the fields
// a newer Instance adds (ADR 0148), while the report this Instance builds carries only the fields
// the schema documents (ADR 0032).
var reportSchema = mustLoadReportSchema()

func mustLoadReportSchema() *openapi3.Schema {
	doc, err := openapi3.NewLoader().LoadFromData(sphericon.TelemetryOpenAPI)
	if err != nil {
		panic(fmt.Sprintf("telemetryschema: load openapi/telemetry.openapi.json: %v", err))
	}
	ref, ok := doc.Components.Schemas["TelemetryReport"]
	if !ok {
		panic("telemetryschema: openapi/telemetry.openapi.json has no TelemetryReport schema")
	}
	closed := false
	ref.Value.AdditionalProperties = openapi3.AdditionalProperties{Has: &closed}
	return ref.Value
}

// Validate reports why body, a JSON-encoded Telemetry report, does not match
// the published schema, or nil when it does.
func Validate(body []byte) error {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return fmt.Errorf("telemetry report is not JSON: %w", err)
	}
	if err := reportSchema.VisitJSON(value); err != nil {
		return fmt.Errorf("telemetry report does not match the published schema: %w", err)
	}
	return nil
}
