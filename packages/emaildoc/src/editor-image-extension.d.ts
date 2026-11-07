// `#editor-image-extension` is the editor's internal image chunk, which build.ts resolves the name
// to: the chunk's file name carries a content hash, and it exports `createImageExtension` as `t`.
// That takes the options of the public `useEditorImage` and returns the same node, so it is typed
// as that hook.
import type { useEditorImage } from '@react-email/editor/plugins'

export declare const t: typeof useEditorImage
