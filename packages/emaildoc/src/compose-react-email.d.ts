// `#compose-react-email` is `composeReactEmail` of @react-email/editor/core, typed for what it reads
// of the editor. The library types it for a whole tiptap Editor, which needs a DOM; it reads only
// these four members, so a render without a browser gives it only them (entry.ts).
import type { composeReactEmail as compose } from '@react-email/editor/core'
import type { Editor } from '@tiptap/core'

/** The members of an editor that `composeReactEmail` reads. */
interface RenderEditor {
  schema: Editor['schema']
  state: { doc: Editor['state']['doc'] }
  extensionManager: { extensions: Editor['extensionManager']['extensions'] }
  getJSON: Editor['getJSON']
}

export declare const composeReactEmail: (
  options: Omit<Parameters<typeof compose>[0], 'editor'> & { editor: RenderEditor },
) => ReturnType<typeof compose>
