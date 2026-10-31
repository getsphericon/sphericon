// @ts-check
// The implementation of the decorators that `typespec/lib/main.tsp` declares. Each one checks
// its arguments here, at `tsp compile` and in the editor, and writes the same `x-*` extension
// the raw `@extension` wrote, so the OpenAPI documents and their Go readers stay as they were.
import { createTypeSpecLibrary, paramMessage } from '@typespec/compiler'
import { getExtensions, setExtension } from '@typespec/openapi'

export const $lib = createTypeSpecLibrary({
  name: 'sphericon',
  diagnostics: {
    'mcp-empty': {
      severity: 'error',
      messages: { default: '@mcp declares nothing: give it `hidden`, `name` or `send`' },
    },
    'mcp-false': {
      severity: 'error',
      messages: {
        default: paramMessage`@mcp declares \`${'key'}: false\`: leave the key out instead`,
      },
    },
    'mcp-hidden-tool': {
      severity: 'error',
      messages: {
        default: paramMessage`@mcp hides the operation and gives it \`${'key'}\`: a hidden operation is no tool`,
      },
    },
    'mcp-name': {
      severity: 'error',
      messages: {
        default: paramMessage`@mcp name \`${'name'}\` is not snake_case (lower-case letters, digits and \`_\`)`,
      },
    },
    'mcp-twice': {
      severity: 'error',
      messages: { default: '@mcp is declared twice on the operation: declare it once' },
    },
  },
})

/** The extension that `internal/mcpserver` reads (ADR 0016). */
const mcpExtension = 'x-mcp'

const snakeCase = /^[a-z][a-z0-9_]*$/

/**
 * @typedef {{ hidden?: boolean, name?: string, send?: boolean }} McpProjection
 */

/**
 * `@mcp`: how an /api operation is projected as an MCP tool (ADR 0016).
 *
 * @param {import('@typespec/compiler').DecoratorContext} context
 * @param {import('@typespec/compiler').Operation} target
 * @param {{ [K in keyof McpProjection]?: McpProjection[K] | undefined }} tool the marshalled
 *   object value, which may carry an absent key as undefined
 */
function $mcp(context, target, tool) {
  const { program } = context
  const report = (/** @type {Parameters<typeof $lib.createDiagnostic>[0]} */ diagnostic) => {
    program.reportDiagnostic($lib.createDiagnostic(diagnostic))
  }
  const at = context.getArgumentTarget(0) ?? target

  if (getExtensions(program, target).has(mcpExtension)) {
    report({ code: 'mcp-twice', target: context.decoratorTarget })
    return
  }
  // The extension leaves an absent key out.
  /** @type {McpProjection} */
  const value = Object.fromEntries(Object.entries(tool).filter(([, v]) => v !== undefined))

  if (Object.keys(value).length === 0) {
    report({ code: 'mcp-empty', target: at })
    return
  }
  for (const key of /** @type {const} */ (['hidden', 'send'])) {
    if (value[key] === false) {
      report({ code: 'mcp-false', format: { key }, target: at })
      return
    }
  }
  if (value.hidden) {
    for (const key of /** @type {const} */ (['name', 'send'])) {
      if (value[key] !== undefined) {
        report({ code: 'mcp-hidden-tool', format: { key }, target: at })
        return
      }
    }
  }
  if (value.name !== undefined && !snakeCase.test(value.name)) {
    report({ code: 'mcp-name', format: { name: value.name }, target: at })
    return
  }
  setExtension(program, target, mcpExtension, value)
}

export const $decorators = {
  SphericonLib: { mcp: $mcp },
}
