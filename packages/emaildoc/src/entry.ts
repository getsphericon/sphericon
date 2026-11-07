// The render path of @react-email/editor without a browser: a document (the editor's JSON) is parsed
// by ProseMirror's own schema, and the editor's own `composeReactEmail` turns it into HTML. Only the
// four members of the editor object that `composeReactEmail` reads are provided, so no DOM is needed.
import { StarterKit } from '@react-email/editor/extensions'
import { EmailTheming } from '@react-email/editor/plugins'
import { getSchemaByResolvedExtensions, resolveExtensions } from '@tiptap/core'

import { composeReactEmail } from '#compose-react-email'
import { t as createImageExtension } from '#editor-image-extension'

type Schema = ReturnType<typeof getSchemaByResolvedExtensions>
type Doc = ReturnType<Schema['nodeFromJSON']>

declare global {
  // What the bundle gives its host: the Go side (internal/kernel/emaildoc) and build.ts.
  var emaildoc: { check: typeof check; render: typeof render; schema: typeof listSchema }
}

// The same extensions the editor UI mounts (StarterKit, the basic theme, the image node). The UI
// passes an upload function to the image node; a render never calls it.
const extensions = resolveExtensions([
  StarterKit.configure(),
  EmailTheming.configure({ theme: 'basic' }),
  createImageExtension({
    uploadImage: async () => Promise.reject(new Error('upload is not available')),
  }),
])
const schema = getSchemaByResolvedExtensions(extensions)

function parse(json: string): Doc {
  return schema.nodeFromJSON(JSON.parse(json))
}

// The editor's containerEnforcer plugin wraps a document that has no container node in one before
// anything is exported (extensions/container.tsx); this does the same.
function withContainer(doc: Doc): Doc {
  const nodes: Doc[] = []
  for (let index = 0; index < doc.childCount; index += 1) {
    nodes.push(doc.child(index))
  }
  if (nodes.some((node) => node.type.name === 'container')) {
    return doc
  }
  const globals = nodes.filter((node) => node.type.name === 'globalContent')
  const rest = nodes.filter((node) => node.type.name !== 'globalContent')
  const content = rest.length > 0 ? rest : [nodeType('paragraph').create()]
  return doc.type.create(doc.attrs, [...globals, nodeType('container').create(null, content)])
}

function nodeType(name: string) {
  const type = schema.nodes[name]
  if (type === undefined) {
    throw new Error(`the editor's schema has no ${name} node`)
  }
  return type
}

// Returns null when the document fits the editor's schema, else ProseMirror's reason.
function check(json: string): string | null {
  try {
    parse(json).check()
    return null
  } catch (error) {
    return error instanceof Error ? error.message : String(error)
  }
}

// Returns a promise of {"html","text"} as a JSON string.
async function render(json: string): Promise<string> {
  const doc = withContainer(parse(json))
  doc.check()
  const editor = {
    schema,
    state: { doc },
    extensionManager: { extensions },
    getJSON: () => doc.toJSON(),
  }
  const { html, text } = await composeReactEmail({ editor })
  return JSON.stringify({ html, text })
}

// What an agent may write: every node and mark with its content expression and its attributes'
// defaults. ProseMirror keeps no attribute types or allowed values, so those are not here: the Go
// side (internal/kernel/emaildoc) adds them to make the contract's listing.
function attrDefaults(attrs: Record<string, { default?: unknown }> | undefined) {
  return Object.fromEntries(
    Object.entries(attrs ?? {}).map(([attr, spec]) => [attr, spec.default ?? null]),
  )
}

function listSchema() {
  const nodes = Object.entries(schema.nodes).map(
    ([name, type]) =>
      [
        name,
        {
          ...(type.spec.content ? { content: type.spec.content } : {}),
          ...(type.spec.group ? { group: type.spec.group } : {}),
          attrs: attrDefaults(type.spec.attrs),
        },
      ] as const,
  )
  const marks = Object.entries(schema.marks).map(
    ([name, type]) => [name, { attrs: attrDefaults(type.spec.attrs) }] as const,
  )
  return { nodes: Object.fromEntries(nodes), marks: Object.fromEntries(marks) }
}

globalThis.emaildoc = { check, render, schema: listSchema }
