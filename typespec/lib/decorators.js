// @ts-check
// The contract decorators (docs/agents/codegen.md, "Contract decorators"). `main.tsp`,
// `common/permissions.tsp`, `common/scopes.tsp` and the Service's `identity.tsp` declare them;
// this file implements them. Each one checks its arguments at `tsp compile` (and in the editor),
// then writes the `x-*` extension the Go readers of the OpenAPI documents read (cmd/scopegen,
// internal/mcpserver), exactly as a raw `@extension` wrote it. `$onValidate` checks what spans
// the contract: what each operation of a surface must declare, and that no raw `@extension`
// writes one of these keys past its decorator.
import {
  createTypeSpecLibrary,
  navigateProgram,
  paramMessage,
  serializeValueAsJson,
} from '@typespec/compiler'
import { getAllHttpServices } from '@typespec/http'
import { $extension, getExtensions, setExtension } from '@typespec/openapi'

/** @import { DecoratorContext, DecoratorApplication, Enum, EnumMember, ModelProperty, Namespace, Operation, Program, Type } from '@typespec/compiler' */

export const $lib = createTypeSpecLibrary({
  name: 'sphericon',
  diagnostics: {
    'raw-extension': {
      severity: 'error',
      messages: {
        default: paramMessage`\`${'key'}\` is written by @${'decorator'}: declare it with the decorator, not a raw @extension`,
      },
    },
    'declared-twice': {
      severity: 'error',
      messages: {
        default: paramMessage`@${'decorator'} is declared twice on the same target: declare it once`,
      },
    },
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
    'granted-outside-grantable': {
      severity: 'error',
      messages: {
        default: '@grantedWith is only for a member of a @grantable enum',
      },
    },
    'scopes-missing': {
      severity: 'error',
      messages: {
        default:
          'The operation declares no scopes: give it @requiresScopes (or @requiresServiceScopes), `#[]` for none',
      },
    },
    'permissions-missing': {
      severity: 'error',
      messages: {
        default:
          'The operation addresses a Workspace and declares no Permissions: give it @requiresPermissions, `#[]` for any member',
      },
    },
    'outside-workspace': {
      severity: 'error',
      messages: {
        default: paramMessage`@${'decorator'} is only for an operation of a Workspace, and this one has no \`workspaceId\` in its path`,
      },
    },
  },
})

/** The extensions the decorators write, by decorator. */
const extensions = /** @type {const} */ ({
  mcp: 'x-mcp',
  requiresPermissions: 'x-required-permissions',
  requiresScopes: 'x-required-scopes',
  requiresServiceScopes: 'x-required-scopes',
  untrusted: 'x-untrusted',
  allowedWhileWithheld: 'x-allowed-while-withheld',
  allowedWhileAcceptancePending: 'x-allowed-while-acceptance-pending',
  grantable: 'x-grant-permissions',
})

/** @typedef {keyof typeof extensions} DecoratorName */

/** The decorator that writes each extension, for the message on a raw `@extension`. */
/** @type {Map<string, string>} */
const decoratorOf = new Map(
  Object.entries(extensions).map(([decorator, key]) => [
    key,
    key === extensions.requiresScopes ? 'requiresScopes or @requiresServiceScopes' : decorator,
  ]),
)

/** The path parameter that addresses a Workspace on /site. */
const workspaceParam = 'workspaceId'

const snakeCase = /^[a-z][a-z0-9_]*$/

/**
 * @param {Program} program
 * @param {Parameters<typeof $lib.createDiagnostic>[0]} diagnostic
 */
function report(program, diagnostic) {
  program.reportDiagnostic($lib.createDiagnostic(diagnostic))
}

/**
 * A decorator argument as the extension's JSON, as `@extension` writes it: a TypeSpec value (an
 * enum member) becomes its JSON, an object or array is converted member by member, and an absent
 * key is left out.
 *
 * @param {Program} program
 * @param {unknown} value
 * @returns {unknown}
 */
function toJson(program, value) {
  if (typeof value !== 'object' || value === null) {
    return value
  }
  if (Array.isArray(value)) {
    return value.map((item) => toJson(program, item))
  }
  if (isTypeSpecValue(value)) {
    return serializeValueAsJson(program, value, value.type)
  }
  return Object.fromEntries(
    Object.entries(value)
      .filter(([, item]) => item !== undefined)
      .map(([key, item]) => [key, toJson(program, item)]),
  )
}

/**
 * Whether a decorator argument is a TypeSpec value left as is by marshalling (an enum member).
 *
 * @param {object} value
 * @returns {value is import('@typespec/compiler').Value}
 */
function isTypeSpecValue(value) {
  return 'entityKind' in value && value.entityKind === 'Value'
}

/**
 * Writes the decorator's extension on the target, unless the target already carries it.
 *
 * @param {DecoratorContext} context
 * @param {Type} target
 * @param {DecoratorName} decorator
 * @param {unknown} value
 */
function declare(context, target, decorator, value) {
  const key = extensions[decorator]
  if (getExtensions(context.program, target).has(key)) {
    report(context.program, {
      code: 'declared-twice',
      format: { decorator },
      target: context.decoratorTarget,
    })
    return
  }
  setExtension(context.program, target, key, value)
}

/**
 * `@mcp`: how an /api operation is projected as an MCP tool (ADR 0016).
 *
 * @param {DecoratorContext} context
 * @param {Operation} target
 * @param {{ hidden?: boolean | undefined, name?: string | undefined, send?: boolean | undefined }} tool
 */
function $mcp(context, target, tool) {
  const { program } = context
  const at = context.getArgumentTarget(0) ?? target
  // The marshalled object carries an absent key as undefined; the extension leaves it out.
  /** @type {{ hidden?: boolean, name?: string, send?: boolean }} */
  const value = {}
  if (tool.hidden !== undefined) {
    value.hidden = tool.hidden
  }
  if (tool.name !== undefined) {
    value.name = tool.name
  }
  if (tool.send !== undefined) {
    value.send = tool.send
  }

  if (Object.keys(value).length === 0) {
    report(program, { code: 'mcp-empty', target: at })
    return
  }
  for (const key of /** @type {const} */ (['hidden', 'send'])) {
    if (value[key] === false) {
      report(program, { code: 'mcp-false', format: { key }, target: at })
      return
    }
  }
  if (value.hidden) {
    for (const key of /** @type {const} */ (['name', 'send'])) {
      if (value[key] !== undefined) {
        report(program, { code: 'mcp-hidden-tool', format: { key }, target: at })
        return
      }
    }
  }
  if (value.name !== undefined && !snakeCase.test(value.name)) {
    report(program, { code: 'mcp-name', format: { name: value.name }, target: at })
    return
  }
  declare(context, target, 'mcp', value)
}

/**
 * `@requiresPermissions`: the Permissions an operation needs (ADR 0061).
 *
 * @param {DecoratorContext} context
 * @param {Operation} target
 * @param {unknown[]} permissions
 */
function $requiresPermissions(context, target, permissions) {
  declare(context, target, 'requiresPermissions', toJson(context.program, permissions))
}

/**
 * `@requiresScopes`: the API token scopes an /api operation needs (ADR 0061).
 *
 * @param {DecoratorContext} context
 * @param {Operation} target
 * @param {unknown[]} scopes
 */
function $requiresScopes(context, target, scopes) {
  declare(context, target, 'requiresScopes', toJson(context.program, scopes))
}

/**
 * `@requiresServiceScopes`: the scopes a /service operation needs (ADR 0026).
 *
 * @param {DecoratorContext} context
 * @param {Operation} target
 * @param {unknown[]} scopes
 */
function $requiresServiceScopes(context, target, scopes) {
  declare(context, target, 'requiresServiceScopes', toJson(context.program, scopes))
}

/**
 * `@untrusted`: the property carries text a caller wrote (ADR 0016).
 *
 * @param {DecoratorContext} context
 * @param {ModelProperty} target
 */
function $untrusted(context, target) {
  declare(context, target, 'untrusted', true)
}

/**
 * `@allowedWhileWithheld`: a User withheld by the Two-factor requirement may call it (ADR 0020).
 *
 * @param {DecoratorContext} context
 * @param {Operation} target
 */
function $allowedWhileWithheld(context, target) {
  declare(context, target, 'allowedWhileWithheld', true)
}

/**
 * `@allowedWhileAcceptancePending`: a User with a required acceptance pending may call it (ADR 0062).
 *
 * @param {DecoratorContext} context
 * @param {Operation} target
 */
function $allowedWhileAcceptancePending(context, target) {
  declare(context, target, 'allowedWhileAcceptancePending', true)
}

/** The @grantable enums, and the Permission of each member that carries @grantedWith. */
const grantableKey = Symbol('sphericon.grantable')
const grantedWithKey = Symbol('sphericon.grantedWith')

/**
 * `@grantable`: a scope enum a User grants to a Connection (ADR 0061). Its `x-grant-permissions`
 * lists every member in order, with its Permission where it carries @grantedWith. The checker
 * finishes an enum's members, their decorators included, before the enum's own decorators, so
 * every @grantedWith has run here; writing it here rather than in `$onValidate` also covers the
 * copy that versioning makes of the enum when /api is emitted.
 *
 * @param {DecoratorContext} context
 * @param {Enum} target
 */
function $grantable(context, target) {
  const { program } = context
  program.stateSet(grantableKey).add(target)
  const granted = program.stateMap(grantedWithKey)
  const rows = [...target.members.values()].map((member) => {
    const scope = toJson(program, member.value ?? member.name)
    const permission = granted.get(member)
    return permission === undefined ? { scope } : { scope, permission }
  })
  declare(context, target, 'grantable', rows)
}

/**
 * `@grantedWith`: the Permission a User must hold to grant this scope.
 *
 * @param {DecoratorContext} context
 * @param {EnumMember} target
 * @param {import('@typespec/compiler').EnumValue} permission
 */
function $grantedWith(context, target, permission) {
  const granted = context.program.stateMap(grantedWithKey)
  if (granted.has(target)) {
    report(context.program, {
      code: 'declared-twice',
      format: { decorator: 'grantedWith' },
      target: context.decoratorTarget,
    })
    return
  }
  granted.set(target, toJson(context.program, permission))
}

/**
 * Refuses a @grantedWith outside a @grantable enum.
 *
 * @param {Program} program
 */
function checkGrantedWith(program) {
  const grantable = program.stateSet(grantableKey)
  for (const [member] of program.stateMap(grantedWithKey)) {
    // An enum that spreads a @grantable one (ApiTokenScope) copies its members and their marks.
    if (member.kind !== 'EnumMember' || grantable.has(member.enum) || isSpreadCopy(member)) {
      continue
    }
    report(program, { code: 'granted-outside-grantable', target: member })
  }
}

/**
 * Whether the member is a copy an enum spread from another (`...GrantableScope`): its node is
 * the original member's, declared in another enum.
 *
 * @param {EnumMember} member
 */
function isSpreadCopy(member) {
  return member.node?.parent !== member.enum.node
}

/** What every operation of a surface must declare, by surface namespace. */
const rulesKey = Symbol('sphericon.everyOperationDeclares')

/**
 * `@everyOperationDeclares`: what each operation of the surface declares. `scopes`: its scopes
 * (/api, /service). `workspacePermissions` (/site): an operation of a Workspace declares its
 * Permissions, and only it declares Permissions or `@allowedWhileWithheld`.
 *
 * @param {DecoratorContext} context
 * @param {Namespace} target
 * @param {'scopes' | 'workspacePermissions'} what
 */
function $everyOperationDeclares(context, target, what) {
  context.program.stateMap(rulesKey).set(target, what)
}

export const $decorators = {
  Sphericon: {
    mcp: $mcp,
    requiresPermissions: $requiresPermissions,
    requiresScopes: $requiresScopes,
    requiresServiceScopes: $requiresServiceScopes,
    untrusted: $untrusted,
    allowedWhileWithheld: $allowedWhileWithheld,
    allowedWhileAcceptancePending: $allowedWhileAcceptancePending,
    grantable: $grantable,
    grantedWith: $grantedWith,
    everyOperationDeclares: $everyOperationDeclares,
  },
}

/**
 * Refuses a raw `@extension` that writes one of the decorators' keys, wherever it stands (a
 * template's operation reports once, at the template).
 *
 * @param {Program} program
 */
function checkRawExtensions(program) {
  const seen = new Set()
  /** @param {Type & { decorators?: DecoratorApplication[] }} type */
  const visit = (type) => {
    for (const application of type.decorators ?? []) {
      if (application.decorator !== $extension) {
        continue
      }
      const key = application.args[0]?.jsValue
      if (typeof key !== 'string') {
        continue
      }
      const decorator = decoratorOf.get(key)
      if (decorator === undefined || seen.has(application.node)) {
        continue
      }
      seen.add(application.node)
      report(program, {
        code: 'raw-extension',
        format: { key, decorator },
        target: application.node ?? type,
      })
    }
  }
  navigateProgram(
    program,
    {
      namespace: visit,
      model: visit,
      modelProperty: visit,
      operation: visit,
      interface: visit,
      enum: visit,
      enumMember: visit,
      union: visit,
      unionVariant: visit,
      scalar: visit,
    },
    { includeTemplateDeclaration: true },
  )
}

/**
 * Checks what every operation of a surface marked with `@everyOperationDeclares` declares.
 *
 * @param {Program} program
 */
function checkSurfaces(program) {
  const rules = program.stateMap(rulesKey)
  if (rules.size === 0) {
    return
  }
  const [services] = getAllHttpServices(program)
  for (const service of services) {
    const what = rules.get(service.namespace)
    for (const httpOperation of what === undefined ? [] : service.operations) {
      const op = httpOperation.operation
      const declared = getExtensions(program, op)
      if (what === 'scopes') {
        if (!declared.has(extensions.requiresScopes)) {
          report(program, { code: 'scopes-missing', target: op })
        }
        continue
      }
      const ofWorkspace = httpOperation.parameters.parameters.some(
        (p) => p.type === 'path' && p.name === workspaceParam,
      )
      if (ofWorkspace) {
        if (!declared.has(extensions.requiresPermissions)) {
          report(program, { code: 'permissions-missing', target: op })
        }
        continue
      }
      for (const decorator of /** @type {const} */ ([
        'requiresPermissions',
        'allowedWhileWithheld',
      ])) {
        if (declared.has(extensions[decorator])) {
          report(program, { code: 'outside-workspace', format: { decorator }, target: op })
        }
      }
    }
  }
}

/** @param {Program} program */
export function $onValidate(program) {
  checkGrantedWith(program)
  checkRawExtensions(program)
  checkSurfaces(program)
}
