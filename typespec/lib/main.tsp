import "./decorators.js";

using TypeSpec.Reflection;

namespace SphericonLib;

/** How an /api operation is projected as an MCP tool (ADR 0016) */
model McpProjection {
  /** The operation is no tool */
  hidden?: boolean;

  /** The tool name in place of the snake-cased operation id */
  name?: string;

  /** A send-class tool: listed and callable only with the `mcp:send` scope (ADR 0065) */
  send?: boolean;
}

/**
 * Declares the operation's `x-mcp` extension, which `internal/mcpserver` reads. The compiler
 * refuses a key that is not in `McpProjection` and a value of the wrong type; the decorator
 * refuses an empty projection, a `false` flag, a hidden operation with a name or `send`, a name
 * that is not snake_case, and a second `@mcp` on the same operation.
 */
extern dec mcp(target: Operation, tool: valueof McpProjection);
