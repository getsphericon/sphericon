import { Input, Loader, Textarea } from '@mantine/core'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { MessageDoc } from '../utils/messageDoc.ts'

// The editor's code is its own chunk: it loads when a message is first edited visually.
const VisualEditor = lazy(() => import('./VisualEditor.tsx'))

// The merge tags every send binds (outbound.Bindings): the Contact's core fields.
const CONTACT_VARIABLES = ['first_name', 'last_name', 'email'] as const

// What an edit leaves behind: the document written in the editor, or the source text of a
// message with no document. A caller stores one and clears the other.
export type MessageEdit = { doc: MessageDoc } | { body: string }

export interface MessageEditorProps {
  // The message as stored: its document, or null for a message that arrived as HTML.
  doc: MessageDoc | null
  // The message's HTML. Shown (and edited) only for a message with no document.
  body: string
  onChange: (edit: MessageEdit) => void
  // The merge tags the picker offers.
  variables?: readonly string[]
}

// The visual editor for a message (ADR 0040), shared by every place that holds one. A message
// with no document and a body opens as editable source and is never loaded into the editor,
// which would drop what it does not model. The mode is decided once, when the editor mounts,
// so clearing the source text does not flip it into the editor mid-typing.
export function MessageEditor({
  doc,
  body,
  onChange,
  variables = CONTACT_VARIABLES,
}: MessageEditorProps) {
  const { t } = useTranslation()
  const [asSource] = useState(() => doc === null && body !== '')
  const [initial] = useState(doc)

  if (asSource) {
    return (
      <Textarea
        label={t(($) => $.messageEditor.sourceLabel)}
        description={t(($) => $.messageEditor.sourceHint)}
        autosize
        minRows={12}
        value={body}
        onChange={(event) => onChange({ body: event.currentTarget.value })}
      />
    )
  }

  return (
    <Input.Wrapper label={t(($) => $.messageEditor.label)}>
      <Suspense fallback={<Loader size="sm" />}>
        <VisualEditor
          initial={initial}
          variables={variables}
          onChange={(next) => onChange({ doc: next })}
        />
      </Suspense>
    </Input.Wrapper>
  )
}
