import { Button, Group, Menu, Stack } from '@mantine/core'
import { EmailEditor } from '@react-email/editor'
import type { EmailEditorRef } from '@react-email/editor'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import type { EditorTheme } from '../utils/brandTheme.ts'
import { docCodec } from '../utils/messageDoc.ts'
import type { MessageDoc } from '../utils/messageDoc.ts'

export interface VisualEditorProps {
  // The document the editor opens with; read once, the editor owns it afterwards.
  initial: MessageDoc | null
  onChange: (doc: MessageDoc) => void
  variables: readonly string[]
  // The editor's theme, read once when the editor mounts: it writes the theme into a document that
  // has none, so it is the Brand kit's palette for a message that is being authored.
  theme: EditorTheme
}

// The only module that imports the editor, so MessageEditor loads it lazily and a page that
// edits no message never pays for it.
export default function VisualEditor({ initial, onChange, variables, theme }: VisualEditorProps) {
  const { t } = useTranslation()
  const editor = useRef<EmailEditorRef>(null)
  const [ready, setReady] = useState(false)

  const insertVariable = (name: string) => {
    editor.current?.editor?.chain().focus().insertContent(`{{ ${name} }}`).run()
  }

  return (
    <Stack gap="xs">
      <Group>
        <Menu>
          <Menu.Target>
            <Button variant="default" size="xs" disabled={!ready}>
              {t(($) => $.messageEditor.insertVariable)}
            </Button>
          </Menu.Target>
          <Menu.Dropdown>
            {variables.map((name) => (
              <Menu.Item key={name} onClick={() => insertVariable(name)}>
                {name}
              </Menu.Item>
            ))}
          </Menu.Dropdown>
        </Menu>
      </Group>
      <EmailEditor
        ref={editor}
        theme={theme}
        {...(initial && { content: initial })}
        placeholder={t(($) => $.messageEditor.placeholder)}
        onReady={() => setReady(true)}
        // Through its JSON text: the editor's theme leaves `undefined` values in the document, which
        // are not JSON and which the API's document object cannot carry.
        onUpdate={(current) => onChange(z.decode(docCodec, JSON.stringify(current.getJSON())))}
      />
    </Stack>
  )
}
