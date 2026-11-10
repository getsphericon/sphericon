// Package bounds is the value bounds of the TypeSpec contract on the input of a domain write (ADR
// 0107): lengths, ranges, item counts and patterns. cmd/boundgen generates it from the OpenAPI
// documents, so the contract stays the one place a bound is written. The domain module that owns
// the field checks each write against the constant, whichever driver made it: HTTP and MCP meet the
// bound twice, in ogen and in the module; the CLI, the jobs, the use cases and the other modules
// meet it in the module. A Go copy of a bound does not exist; Owners names who checks each one.
package bounds
