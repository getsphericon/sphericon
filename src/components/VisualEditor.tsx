import { Button, Group, Menu, Paper, Stack } from '@mantine/core'
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

  // Opening the menu moves the editor's selection to the start of the message, so the caret is
  // remembered while the editor has focus and the tag goes there.
  const caret = useRef<number | null>(null)

  const insertVariable = (name: string) => {
    const current = editor.current?.editor
    const tag = `{{ ${name} }}`
    if (caret.current === null) {
      current?.chain().focus().insertContent(tag).run()
    } else {
      current?.chain().insertContentAt(caret.current, tag).focus().run()
    }
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
      {/* The editor draws only the message, which is white in the basic theme, so the page gives it a
          frame: a white sheet with a minimum height, dark default text (the editor inherits the app's
          light text in the dark scheme), and a click anywhere on it puts the caret at the end. */}
      <Paper
        withBorder
        p="md"
        mih={320}
        bg="white"
        c="black"
        onClick={() => {
          editor.current?.editor?.commands.focus('end')
        }}
      >
        <EmailEditor
          ref={editor}
          theme={theme}
          {...(initial && { content: initial })}
          placeholder={t(($) => $.messageEditor.placeholder)}
          onReady={(current) => {
            current.editor?.on('selectionUpdate', ({ editor: tiptap }) => {
              if (tiptap.isFocused) {
                caret.current = tiptap.state.selection.to
              }
            })
            setReady(true)
          }}
          // Through its JSON text: the editor's theme leaves `undefined` values in the document, which
          // are not JSON and which the API's document object cannot carry.
          onUpdate={(current) => onChange(z.decode(docCodec, JSON.stringify(current.getJSON())))}
        />
      </Paper>
    </Stack>
  )
}
