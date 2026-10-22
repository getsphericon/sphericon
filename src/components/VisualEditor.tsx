import {
  ActionIcon,
  Button,
  Divider,
  Group,
  Menu,
  Paper,
  Popover,
  Stack,
  TextInput,
  Tooltip,
} from '@mantine/core'
import { EmailEditor } from '@react-email/editor'
import type { EmailEditorRef } from '@react-email/editor'
import {
  IconArrowBackUp,
  IconArrowForwardUp,
  IconBold,
  IconH1,
  IconH2,
  IconItalic,
  IconLink,
  IconList,
  IconListNumbers,
  IconQuote,
  IconStrikethrough,
  IconUnderline,
} from '@tabler/icons-react'
import type { ComponentType } from 'react'
import { useEffect, useReducer, useRef, useState } from 'react'
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

type Editor = NonNullable<EmailEditorRef['editor']>

interface ToolbarButton {
  label: string
  Icon: ComponentType<{ size?: number }>
  active: (editor: Editor) => boolean
  run: (editor: Editor) => void
  enabled?: (editor: Editor) => boolean
}

function EditorToolbar({ editor }: { editor: Editor }) {
  const { t } = useTranslation()
  // The toolbar shows the state of the selection, so it redraws on every transaction.
  const [, redraw] = useReducer((n: number) => n + 1, 0)
  useEffect(() => {
    editor.on('transaction', redraw)
    return () => {
      editor.off('transaction', redraw)
    }
  }, [editor])

  const [linkOpen, setLinkOpen] = useState(false)
  const [href, setHref] = useState('')

  const groups: ToolbarButton[][] = [
    [
      {
        label: t(($) => $.messageEditor.toolbar.undo),
        Icon: IconArrowBackUp,
        active: () => false,
        enabled: (e) => e.can().undo(),
        run: (e) => {
          e.chain().focus().undo().run()
        },
      },
      {
        label: t(($) => $.messageEditor.toolbar.redo),
        Icon: IconArrowForwardUp,
        active: () => false,
        enabled: (e) => e.can().redo(),
        run: (e) => {
          e.chain().focus().redo().run()
        },
      },
    ],
    [
      {
        label: t(($) => $.messageEditor.toolbar.heading1),
        Icon: IconH1,
        active: (e) => e.isActive('heading', { level: 1 }),
        run: (e) => {
          e.chain().focus().toggleHeading({ level: 1 }).run()
        },
      },
      {
        label: t(($) => $.messageEditor.toolbar.heading2),
        Icon: IconH2,
        active: (e) => e.isActive('heading', { level: 2 }),
        run: (e) => {
          e.chain().focus().toggleHeading({ level: 2 }).run()
        },
      },
    ],
    [
      {
        label: t(($) => $.messageEditor.toolbar.bold),
        Icon: IconBold,
        active: (e) => e.isActive('bold'),
        run: (e) => {
          e.chain().focus().toggleBold().run()
        },
      },
      {
        label: t(($) => $.messageEditor.toolbar.italic),
        Icon: IconItalic,
        active: (e) => e.isActive('italic'),
        run: (e) => {
          e.chain().focus().toggleItalic().run()
        },
      },
      {
        label: t(($) => $.messageEditor.toolbar.underline),
        Icon: IconUnderline,
        active: (e) => e.isActive('underline'),
        run: (e) => {
          e.chain().focus().toggleUnderline().run()
        },
      },
      {
        label: t(($) => $.messageEditor.toolbar.strike),
        Icon: IconStrikethrough,
        active: (e) => e.isActive('strike'),
        run: (e) => {
          e.chain().focus().toggleStrike().run()
        },
      },
    ],
    [
      {
        label: t(($) => $.messageEditor.toolbar.bulletList),
        Icon: IconList,
        active: (e) => e.isActive('bulletList'),
        run: (e) => {
          e.chain().focus().toggleBulletList().run()
        },
      },
      {
        label: t(($) => $.messageEditor.toolbar.orderedList),
        Icon: IconListNumbers,
        active: (e) => e.isActive('orderedList'),
        run: (e) => {
          e.chain().focus().toggleOrderedList().run()
        },
      },
      {
        label: t(($) => $.messageEditor.toolbar.quote),
        Icon: IconQuote,
        active: (e) => e.isActive('blockquote'),
        run: (e) => {
          e.chain().focus().toggleBlockquote().run()
        },
      },
    ],
  ]

  const applyLink = () => {
    const chain = editor.chain().focus().extendMarkRange('link')
    if (href.trim() === '') {
      chain.unsetLink().run()
    } else {
      chain.setLink({ href: href.trim() }).run()
    }
    setLinkOpen(false)
  }

  return (
    <Group gap={4} role="toolbar" aria-label={t(($) => $.messageEditor.toolbar.label)}>
      {groups.map((group, index) => (
        <Group key={group[0]?.label} gap={2}>
          {index > 0 ? <Divider orientation="vertical" mr={2} /> : null}
          {group.map(({ label, Icon, active, enabled, run }) => (
            <Tooltip key={label} label={label}>
              <ActionIcon
                variant={active(editor) ? 'filled' : 'default'}
                aria-label={label}
                aria-pressed={active(editor)}
                disabled={enabled ? !enabled(editor) : false}
                // Keeps the selection in the editor when the button is pressed.
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => run(editor)}
              >
                <Icon size={16} />
              </ActionIcon>
            </Tooltip>
          ))}
        </Group>
      ))}
      <Divider orientation="vertical" />
      <Popover opened={linkOpen} onChange={setLinkOpen} withArrow>
        <Popover.Target>
          <Tooltip label={t(($) => $.messageEditor.toolbar.link)}>
            <ActionIcon
              variant={editor.isActive('link') ? 'filled' : 'default'}
              aria-label={t(($) => $.messageEditor.toolbar.link)}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => {
                setHref(String(editor.getAttributes('link').href ?? ''))
                setLinkOpen((open) => !open)
              }}
            >
              <IconLink size={16} />
            </ActionIcon>
          </Tooltip>
        </Popover.Target>
        <Popover.Dropdown>
          <Group gap="xs" align="flex-end">
            <TextInput
              size="xs"
              aria-label={t(($) => $.messageEditor.toolbar.linkUrl)}
              placeholder="https://"
              value={href}
              onChange={(event) => setHref(event.currentTarget.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  applyLink()
                }
              }}
            />
            <Button size="xs" onClick={applyLink}>
              {t(($) => $.messageEditor.toolbar.linkApply)}
            </Button>
          </Group>
        </Popover.Dropdown>
      </Popover>
    </Group>
  )
}

// The only module that imports the editor, so MessageEditor loads it lazily and a page that
// edits no message never pays for it.
export default function VisualEditor({ initial, onChange, variables, theme }: VisualEditorProps) {
  const { t } = useTranslation()
  const editor = useRef<EmailEditorRef>(null)
  const [ready, setReady] = useState(false)
  const [tiptap, setTiptap] = useState<Editor | null>(null)

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
      {tiptap ? <EditorToolbar editor={tiptap} /> : null}
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
            current.editor?.on('selectionUpdate', ({ editor: instance }) => {
              if (instance.isFocused) {
                caret.current = instance.state.selection.to
              }
            })
            setTiptap(current.editor)
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
