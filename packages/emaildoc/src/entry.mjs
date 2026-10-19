// The render path of @react-email/editor without a browser: a document (the editor's JSON) is parsed
// by ProseMirror's own schema, and the editor's own `composeReactEmail` turns it into HTML. Only the
// four members of the editor object that `composeReactEmail` reads are provided, so no DOM is needed.
import { composeReactEmail } from '@react-email/editor/core'
import { StarterKit } from '@react-email/editor/extensions'
import { EmailTheming } from '@react-email/editor/plugins'
import { getSchemaByResolvedExtensions, resolveExtensions } from '@tiptap/core'
import { t as createImageExtension } from 'editor-image-extension'

// The same extensions the editor UI mounts (StarterKit, the basic theme, the image node). The UI
// passes an upload function to the image node; a render never calls it.
const extensions = resolveExtensions([
  StarterKit.configure(),
  EmailTheming.configure({ theme: 'basic' }),
  createImageExtension({ uploadImage: () => Promise.reject(new Error('upload is not available')) }),
])
const schema = getSchemaByResolvedExtensions(extensions)

function parse(json) {
  return schema.nodeFromJSON(JSON.parse(json))
}

// The editor's containerEnforcer plugin wraps a document that has no container node in one before
// anything is exported (extensions/container.tsx); this does the same.
function withContainer(doc) {
  const nodes = []
  doc.forEach((node) => {
    nodes.push(node)
  })
  if (nodes.some((node) => node.type.name === 'container')) {
    return doc
  }
  const globals = nodes.filter((node) => node.type.name === 'globalContent')
  const rest = nodes.filter((node) => node.type.name !== 'globalContent')
  const content = rest.length > 0 ? rest : [schema.nodes.paragraph.create()]
  return doc.type.create(doc.attrs, [...globals, schema.nodes.container.create(null, content)])
}

// Returns null when the document fits the editor's schema, else ProseMirror's reason.
function check(json) {
  try {
    parse(json).check()
    return null
  } catch (error) {
    return error instanceof Error ? error.message : String(error)
  }
}

// Returns a promise of {"html","text"} as a JSON string.
async function render(json, preview) {
  const doc = withContainer(parse(json))
  doc.check()
  const editor = {
    schema,
    state: { doc },
    extensionManager: { extensions },
    getJSON: () => doc.toJSON(),
  }
  const { html, text } = await composeReactEmail({ editor, preview })
  return JSON.stringify({ html, text })
}

// What an agent may write: every node and mark with its content expression and its attributes'
// defaults. ProseMirror keeps no attribute types or allowed values, so those are not here.
function describe(types, kind) {
  return Object.fromEntries(
    Object.entries(types).map(([name, type]) => {
      const entry = {}
      if (kind === 'node') {
        if (type.spec.content) {
          entry.content = type.spec.content
        }
        if (type.spec.group) {
          entry.group = type.spec.group
        }
      }
      entry.attrs = Object.fromEntries(
        Object.entries(type.spec.attrs ?? {}).map(([attr, spec]) => [attr, spec.default ?? null]),
      )
      return [name, entry]
    }),
  )
}

function listSchema() {
  return { nodes: describe(schema.nodes, 'node'), marks: describe(schema.marks, 'mark') }
}

globalThis.emaildoc = { check, render, schema: listSchema }
