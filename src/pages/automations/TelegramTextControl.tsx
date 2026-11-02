import { Group, Menu, Stack, Text, Textarea } from '@mantine/core'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { CONTACT_TAGS, EVENT_ACTION_TAG } from '../../utils/mergeTags.ts'
import { TELEGRAM_TEXT_LIMIT } from './telegramFormat.ts'

// The merge tags a Telegram step binds, as an email step's message does: the Contact's fields and
// the Event the Enrollment began on.
const TAGS = [...CONTACT_TAGS, EVENT_ACTION_TAG]

// TelegramTextControl edits a Telegram step's text: Telegram's HTML subset with Liquid merge tags,
// a picker that inserts a tag where the cursor is, and a counter of the characters as written.
export function TelegramTextControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()
  const field = useRef<HTMLTextAreaElement>(null)
  const text = typeof data === 'string' ? data : ''
  const over = text.length > TELEGRAM_TEXT_LIMIT

  const insert = (tag: string) => {
    const at = field.current?.selectionStart ?? text.length
    const end = field.current?.selectionEnd ?? at
    props.handleChange(path, `${text.slice(0, at)}{{ ${tag} }}${text.slice(end)}`)
  }

  return (
    <Stack gap="xs">
      <Textarea
        ref={field}
        label={label}
        description={t(($) => $.automations.telegramTextHint)}
        autosize
        minRows={4}
        maxRows={10}
        value={text}
        error={over ? t(($) => $.automations.telegramTextTooLong) : undefined}
        onChange={(e) => props.handleChange(path, e.currentTarget.value)}
      />
      <Group justify="space-between">
        <Menu>
          <Menu.Target>
            <Text component="button" type="button" size="sm" c="blue">
              {t(($) => $.automations.insertMergeTag)}
            </Text>
          </Menu.Target>
          <Menu.Dropdown>
            {TAGS.map((tag) => (
              <Menu.Item key={tag} onClick={() => insert(tag)}>
                {tag}
              </Menu.Item>
            ))}
          </Menu.Dropdown>
        </Menu>
        <Text size="sm" c={over ? 'red' : 'dimmed'}>
          {t(($) => $.automations.telegramCounter, {
            length: text.length,
            limit: TELEGRAM_TEXT_LIMIT,
          })}
        </Text>
      </Group>
    </Stack>
  )
}
