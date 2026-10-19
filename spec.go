// Package sphericon holds repository-root assets that Go code embeds.
package sphericon

import _ "embed"

// ExternalOpenAPI is the generated external (/api) OpenAPI document. The MCP
// surface (internal/mcpserver) projects its tools from it, so the tool list can
// never drift from the contract (ADR 0016).
//
//go:embed openapi/external.openapi.json
var ExternalOpenAPI []byte

// TelemetryOpenAPI is the generated Telemetry report OpenAPI document (ADR 0032),
// the published contract every built report is validated against
// (internal/telemetryschema).
//
//go:embed openapi/telemetry.openapi.json
var TelemetryOpenAPI []byte
