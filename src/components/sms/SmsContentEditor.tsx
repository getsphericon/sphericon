import { Group, Menu, Stack, Text, Textarea } from '@mantine/core'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { CONTACT_TAGS, UNSUBSCRIBE_URL_TAG } from '../../utils/mergeTags.ts'
import { measureSms } from '../../utils/smsParts.ts'

interface Props {
  value: string
  onChange: (next: string) => void
  // Merge tags of the surface beyond the Contact's (an Automation adds the Event's).
  extraTags?: readonly string[]
}

const NO_TAGS: readonly string[] = []

// SmsContentEditor edits an SMS (ADR 0078): one plain text with a merge-tag picker. Nothing is
// appended to it, so an opt-out line is the author's to write, with `unsubscribe_url` if they want
// a link. A counter shows the text's characters, encoding and segments as written; the provider's
// part limit is the save's to apply. It holds no state of its own.
export function SmsContentEditor({ value, onChange, extraTags = NO_TAGS }: Props) {
  const { t } = useTranslation()
  const field = useRef<HTMLTextAreaElement>(null)
  const tags = [...CONTACT_TAGS, ...extraTags, UNSUBSCRIBE_URL_TAG]
  const size = measureSms(value)

  const insert = (tag: string) => {
    const at = field.current?.selectionStart ?? value.length
    const end = field.current?.selectionEnd ?? at
    onChange(`${value.slice(0, at)}{{ ${tag} }}${value.slice(end)}`)
  }

  return (
    <Stack gap="xs">
      <Textarea
        ref={field}
        label={t(($) => $.smsEditor.textLabel)}
        description={t(($) => $.smsEditor.textHint)}
        autosize
        minRows={4}
        maxRows={10}
        value={value}
        onChange={(e) => onChange(e.currentTarget.value)}
      />
      <Group justify="space-between">
        <Menu>
          <Menu.Target>
            <Text component="button" type="button" size="sm" c="blue">
              {t(($) => $.smsEditor.insertMergeTag)}
            </Text>
          </Menu.Target>
          <Menu.Dropdown>
            {tags.map((tag) => (
              <Menu.Item key={tag} onClick={() => insert(tag)}>
                {tag}
              </Menu.Item>
            ))}
          </Menu.Dropdown>
        </Menu>
        <Text size="sm" c="dimmed">
          {t(($) => $.smsEditor.counter, {
            characters: t(($) => $.smsEditor.characters, { count: size.characters }),
            encoding: size.encoding,
            segments: t(($) => $.smsEditor.segments, { count: size.parts }),
          })}
        </Text>
      </Group>
    </Stack>
  )
}
