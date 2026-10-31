import { Input, Textarea } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { siteBrandKitGetOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import { Route as workspaceRoute } from '../routes/workspaces/$workspaceId/route.tsx'
import { brandTheme } from '../utils/brandTheme.ts'
import { CONTACT_TAGS, EVENT_ACTION_TAG } from '../utils/mergeTags.ts'
import type { MessageDoc } from '../utils/messageDoc.ts'
import { BlockSkeleton } from './Skeletons.tsx'

// The editor's code is its own chunk: it loads when a message is first edited visually.
const VisualEditor = lazy(async () =>
  import('./VisualEditor.tsx').then((m) => ({ default: m.VisualEditor })),
)

// What an edit leaves behind: the document written in the editor, or the source text of a
// message with no document. A caller stores one and clears the other.
type MessageEdit = { doc: MessageDoc } | { body: string }

export interface MessageEditorProps {
  // The message as stored: its document, or null for a message that arrived as HTML.
  doc: MessageDoc | null
  // The message's HTML. Shown (and edited) only for a message with no document.
  body: string
  onChange: (edit: MessageEdit) => void
  // An Automation's message: it also binds `event`, the Event its Enrollment began on (#752), so the
  // picker offers `event.action` and a property of that Event by name.
  eventTags?: boolean
}

// The visual editor for a message (ADR 0040), shared by every place that holds one. A message
// with no document and a body opens as editable source and is never loaded into the editor,
// which would drop what it does not model. The mode is decided once, when the editor mounts,
// so clearing the source text does not flip it into the editor mid-typing.
export function MessageEditor({ doc, body, onChange, eventTags = false }: MessageEditorProps) {
  const { t } = useTranslation()
  const [asSource] = useState(() => doc === null && body !== '')
  const [initial] = useState(doc)
  const { workspaceId } = workspaceRoute.useParams()
  // Only a new message is authored in the Brand kit's palette.
  const isNew = !asSource && initial === null
  const brandKit = useQuery({
    ...siteBrandKitGetOptions({ path: { workspaceId } }),
    enabled: isNew,
  })

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

  // The Brand kit is written into a message when the message is authored (ADR 0040): a new message
  // opens with its palette, an existing one keeps the look it was written with. The editor reads
  // its theme once, on mount, so a new message waits for the kit.
  const theme = brandTheme(isNew ? brandKit.data : undefined)

  return (
    <Input.Wrapper label={t(($) => $.messageEditor.label)}>
      {brandKit.isLoading ? (
        <BlockSkeleton />
      ) : (
        <Suspense fallback={<BlockSkeleton />}>
          <VisualEditor
            initial={initial}
            variables={eventTags ? [...CONTACT_TAGS, EVENT_ACTION_TAG] : CONTACT_TAGS}
            eventProperties={eventTags}
            theme={theme}
            onChange={(next) => onChange({ doc: next })}
          />
        </Suspense>
      )}
    </Input.Wrapper>
  )
}
