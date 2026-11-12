// The contract decorators (docs/agents/codegen.md, "Contract decorators"). `main.tsp`,
// `common/permissions.tsp`, `common/scopes.tsp` and the Service's `identity.tsp` declare them;
// this file implements them. Each one checks its arguments at `tsp compile` (and in the editor),
// then writes the `x-*` extension the Go readers of the OpenAPI documents read (cmd/scopegen,
// internal/mcpserver), exactly as a raw `@extension` wrote it. `$onValidate` checks what spans
// the contract: what each operation of a surface must declare, that no raw `@extension` writes
// one of these keys past its decorator, and that no description names the code behind it.
import {
  createTypeSpecLibrary,
  getDocData,
  getErrorsDocData,
  getReturnsDocData,
  getSourceLocation,
  getSummary,
  navigateProgram,
  paramMessage,
  serializeValueAsJson,
} from '@typespec/compiler'
import type {
  DecoratorApplication,
  DecoratorContext,
  Enum,
  EnumMember,
  EnumValue,
  ModelProperty,
  Namespace,
  Operation,
  Program,
  Type,
  Value,
} from '@typespec/compiler'
import { getAllHttpServices } from '@typespec/http'
import { $extension, getExtensions, setExtension } from '@typespec/openapi'

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
    'implementation-in-description': {
      severity: 'error',
      messages: {
        default: paramMessage`The description names the code behind the contract (\`${'word'}\`): say what happens to the value, in the words of the glossary, not which code does it`,
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
const extensions = {
  mcp: 'x-mcp',
  requiresPermissions: 'x-required-permissions',
  requiresScopes: 'x-required-scopes',
  requiresServiceScopes: 'x-required-scopes',
  untrusted: 'x-untrusted',
  allowedWhileWithheld: 'x-allowed-while-withheld',
  allowedWhileAcceptancePending: 'x-allowed-while-acceptance-pending',
  grantable: 'x-grant-permissions',
  rateLimited: 'x-rate-limit',
} as const

type DecoratorName = keyof typeof extensions

/** The decorator that writes each extension, for the message on a raw `@extension`. */
const decoratorOf = new Map<string, string>(
  Object.entries(extensions).map(([decorator, key]) => [
    key,
    key === extensions.requiresScopes ? 'requiresScopes or @requiresServiceScopes' : decorator,
  ]),
)

/** The path parameter that addresses a Workspace on /site. */
const workspaceParam = 'workspaceId'

const snakeCase = /^[a-z][a-z0-9_]*$/

/** A declaration that can carry a decorator. */
type Decorated = Type & { decorators?: DecoratorApplication[] }

function report(program: Program, diagnostic: Parameters<typeof $lib.createDiagnostic>[0]) {
  program.reportDiagnostic($lib.createDiagnostic(diagnostic))
}

/**
 * A decorator argument as the extension's JSON, as `@extension` writes it: a TypeSpec value (an
 * enum member) becomes its JSON, an object or array is converted member by member, and an absent
 * key is left out.
 */
function toJson(program: Program, value: unknown): unknown {
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

/** Whether a decorator argument is a TypeSpec value left as is by marshalling (an enum member). */
function isTypeSpecValue(value: object): value is Value {
  return 'entityKind' in value && value.entityKind === 'Value'
}

/** An extension a decorator writes: which decorator, and the value it writes. */
interface Declaration {
  decorator: DecoratorName
  value: unknown
}

/** Writes the decorator's extension on the target, unless the target already carries it. */
function declare(context: DecoratorContext, target: Type, { decorator, value }: Declaration) {
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

/** The keys of `@mcp`; the marshalled object carries an absent key as undefined. */
interface McpTool {
  hidden?: boolean | undefined
  name?: string | undefined
  send?: boolean | undefined
}

/** `@mcp`: how an /api operation is projected as an MCP tool (ADR 0016). */
function $mcp(context: DecoratorContext, target: Operation, tool: McpTool) {
  const { program } = context
  const at = context.getArgumentTarget(0) ?? target
  // The extension leaves an absent key out.
  const value: { hidden?: boolean; name?: string; send?: boolean } = {}
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
  for (const key of ['hidden', 'send'] as const) {
    if (value[key] === false) {
      report(program, { code: 'mcp-false', format: { key }, target: at })
      return
    }
  }
  if (value.hidden) {
    for (const key of ['name', 'send'] as const) {
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
  declare(context, target, { decorator: 'mcp', value })
}

/** `@requiresPermissions`: the Permissions an operation needs (ADR 0061). */
function $requiresPermissions(
  context: DecoratorContext,
  target: Operation,
  permissions: unknown[],
) {
  declare(context, target, {
    decorator: 'requiresPermissions',
    value: toJson(context.program, permissions),
  })
}

/** `@requiresScopes`: the API token scopes an /api operation needs (ADR 0061). */
function $requiresScopes(context: DecoratorContext, target: Operation, scopes: unknown[]) {
  declare(context, target, { decorator: 'requiresScopes', value: toJson(context.program, scopes) })
}

/** `@requiresServiceScopes`: the scopes a /service operation needs (ADR 0026). */
function $requiresServiceScopes(context: DecoratorContext, target: Operation, scopes: unknown[]) {
  declare(context, target, {
    decorator: 'requiresServiceScopes',
    value: toJson(context.program, scopes),
  })
}

/** `@untrusted`: the property carries text a caller wrote (ADR 0016). */
function $untrusted(context: DecoratorContext, target: ModelProperty) {
  declare(context, target, { decorator: 'untrusted', value: true })
}

/** `@allowedWhileWithheld`: a User withheld by the Two-factor requirement may call it (ADR 0020). */
function $allowedWhileWithheld(context: DecoratorContext, target: Operation) {
  declare(context, target, { decorator: 'allowedWhileWithheld', value: true })
}

/**
 * `@allowedWhileAcceptancePending`: a User with a required acceptance pending may call it (ADR 0062).
 */
function $allowedWhileAcceptancePending(context: DecoratorContext, target: Operation) {
  declare(context, target, { decorator: 'allowedWhileAcceptancePending', value: true })
}

/**
 * `@rateLimited`: the server limits the operation per client address under the policy: `login`, one
 * budget every step of a login shares, `forgot_password`, the reset requests', or `human`, a budget
 * of its own for each public form a person submits.
 */
function $rateLimited(
  context: DecoratorContext,
  target: Operation,
  policy: 'login' | 'forgot_password' | 'human',
) {
  declare(context, target, { decorator: 'rateLimited', value: policy })
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
 */
function $grantable(context: DecoratorContext, target: Enum) {
  const { program } = context
  program.stateSet(grantableKey).add(target)
  const granted = program.stateMap(grantedWithKey)
  const rows = [...target.members.values()].map((member) => {
    const scope = toJson(program, member.value ?? member.name)
    const permission = granted.get(member)
    return permission === undefined ? { scope } : { scope, permission }
  })
  declare(context, target, { decorator: 'grantable', value: rows })
}

/** `@grantedWith`: the Permission a User must hold to grant this scope. */
function $grantedWith(context: DecoratorContext, target: EnumMember, permission: EnumValue) {
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

/** Refuses a @grantedWith outside a @grantable enum. */
function checkGrantedWith(program: Program) {
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
 */
function isSpreadCopy(member: EnumMember) {
  return member.node?.parent !== member.enum.node
}

/** What every operation of a surface must declare, by surface namespace. */
const rulesKey = Symbol('sphericon.everyOperationDeclares')

/**
 * `@everyOperationDeclares`: what each operation of the surface declares. `scopes`: its scopes
 * (/api, /service). `workspacePermissions` (/site): an operation of a Workspace declares its
 * Permissions, and only it declares Permissions or `@allowedWhileWithheld`.
 */
function $everyOperationDeclares(
  context: DecoratorContext,
  target: Namespace,
  what: 'scopes' | 'workspacePermissions',
) {
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
    rateLimited: $rateLimited,
    grantable: $grantable,
    grantedWith: $grantedWith,
    everyOperationDeclares: $everyOperationDeclares,
  },
}

/**
 * Calls `visit` on every declaration of the program that can carry a decorator, a template's
 * declaration included.
 */
function visitDeclarations(program: Program, visit: (type: Decorated) => void) {
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
 * Refuses a raw `@extension` that writes one of the decorators' keys, wherever it stands (a
 * template's operation reports once, at the template).
 */
function checkRawExtensions(program: Program) {
  const seen = new Set<unknown>()
  const visit = (type: Decorated) => {
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
  visitDeclarations(program, visit)
}

/**
 * The descriptions of a type, by what they describe: the type (a doc comment or `@doc`), its
 * summary, and an operation's return and errors (`@returns` or `@returnsDoc`, `@errors` or
 * `@errorsDoc`). Each reads the text the OpenAPI document carries, however it was written.
 */
const descriptions: Record<string, (program: Program, type: Type) => string | undefined> = {
  doc: (program, type) => getDocData(program, type)?.value,
  summary: (program, type) => getSummary(program, type),
  returns: (program, type) =>
    type.kind === 'Operation' ? getReturnsDocData(program, type)?.value : undefined,
  errors: (program, type) =>
    type.kind === 'Operation' ? getErrorsDocData(program, type)?.value : undefined,
}

/**
 * What names the code behind the contract rather than says what happens: a module, a package or a
 * use case; a Go identifier qualified by its package (`throttle.NormalizeEmail`, while
 * `window.Sphericon.track()` is a path of a script); a Go source path or file.
 */
const implementationWords = [
  /\b(?:modules?|packages?|use[ -]cases?)\b/i,
  /\b[a-z][a-z0-9]*\.[A-Z][A-Za-z0-9]*(?![\w.])/,
  /\b(?:internal|ee|cmd)\/[a-z]\w*/,
  /\b\w+\.go\b/,
]

/** The first words of a text that name the code behind the contract, or undefined. */
function implementationWord(text: string): string | undefined {
  for (const word of implementationWords) {
    const match = word.exec(text)
    if (match !== null) {
      return match[0]
    }
  }
  return undefined
}

/**
 * What a type was copied from: the operation or model it `is`, the property it spreads or a
 * template declares.
 */
function copiedFrom(type: Type): Type | undefined {
  if (type.kind === 'Operation') {
    return type.sourceOperation
  }
  if (type.kind === 'Model') {
    return type.sourceModel
  }
  if (type.kind === 'ModelProperty') {
    return type.sourceProperty
  }
  return undefined
}

/**
 * The type a description was written on. An operation or a model declared with `is`, and a
 * property spread or copied from a template, carry the description of their source: it is
 * reported once, at the source.
 */
function writtenOn(type: Type, read: (type: Type) => string | undefined): Type {
  const text = read(type)
  let at = type
  for (;;) {
    const source = copiedFrom(at)
    if (source === undefined || read(source) !== text) {
      return at
    }
    at = source
  }
}

/**
 * Refuses a description that names the code behind the contract: it goes stale when the code
 * moves, and a client cannot act on it. A description says what happens to the value, in the
 * glossary's words. Each is reported once, where it is written; a library's are not read.
 */
function checkDescriptions(program: Program) {
  const seen = new Map<string, Set<unknown>>()
  visitDeclarations(program, (type) => {
    for (const [kind, describe] of Object.entries(descriptions)) {
      const read = (t: Type) => describe(program, t)
      const text = read(type)
      const word = text === undefined ? undefined : implementationWord(text)
      if (word === undefined) {
        continue
      }
      const at = writtenOn(type, read)
      const key = ('node' in at ? at.node : undefined) ?? at
      const reported = seen.get(kind) ?? new Set()
      seen.set(kind, reported)
      const inLibrary = getSourceLocation(at).file.path.includes('/node_modules/')
      if (reported.has(key) || inLibrary) {
        continue
      }
      reported.add(key)
      report(program, { code: 'implementation-in-description', format: { word }, target: at })
    }
  })
}

/** Checks what every operation of a surface marked with `@everyOperationDeclares` declares. */
function checkSurfaces(program: Program) {
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
      for (const decorator of ['requiresPermissions', 'allowedWhileWithheld'] as const) {
        if (declared.has(extensions[decorator])) {
          report(program, { code: 'outside-workspace', format: { decorator }, target: op })
        }
      }
    }
  }
}

export function $onValidate(program: Program) {
  checkGrantedWith(program)
  checkRawExtensions(program)
  checkSurfaces(program)
  checkDescriptions(program)
}
