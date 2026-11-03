import {
  ActionIcon,
  Button,
  Group,
  Menu,
  Stack,
  Switch,
  Text,
  Textarea,
  TextInput,
} from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import type { ReactNode } from 'react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { CONTACT_TAGS, TELEGRAM_START_TAG, UNSUBSCRIBE_URL_TAG } from '../../utils/mergeTags.ts'
import type { TelegramContentValue } from '../../utils/telegramContent.ts'
import { TELEGRAM_MAX_BUTTONS, telegramLimit } from '../../utils/telegramContent.ts'
import { TelegramPreview } from './TelegramPreview.tsx'

interface Props {
  value: TelegramContentValue
  onChange: (next: TelegramContentValue) => void
  // Merge tags of the surface beyond the Contact's (an Automation adds the Event's).
  extraTags?: readonly string[]
  // Controls of the content that other tickets own (the unsubscribe button), shown after the
  // link-preview toggle.
  children?: ReactNode
}

// TelegramContentEditor edits a Telegram message (ADR 0078): the text with a merge-tag picker, an
// image URL, the link buttons, the link-preview toggle, a counter that knows which limit applies
// and a preview for a sample Contact. The Automation step and the Broadcast use it; it holds no
// state of its own.
const NO_TAGS: readonly string[] = []

export function TelegramContentEditor({ value, onChange, extraTags = NO_TAGS, children }: Props) {
  const { t } = useTranslation()
  const field = useRef<HTMLTextAreaElement>(null)
  const limit = telegramLimit(value.imageUrl)
  const over = value.text.length > limit
  const tags = [...CONTACT_TAGS, TELEGRAM_START_TAG, ...extraTags, UNSUBSCRIBE_URL_TAG]
  const patch = (next: Partial<TelegramContentValue>) => onChange({ ...value, ...next })

  const insert = (tag: string) => {
    const at = field.current?.selectionStart ?? value.text.length
    const end = field.current?.selectionEnd ?? at
    patch({ text: `${value.text.slice(0, at)}{{ ${tag} }}${value.text.slice(end)}` })
  }
  const setButton = (index: number, next: Partial<TelegramContentValue['buttons'][number]>) =>
    patch({ buttons: value.buttons.map((b, i) => (i === index ? { ...b, ...next } : b)) })

  return (
    <Stack gap="sm">
      <Stack gap="xs">
        <Textarea
          ref={field}
          label={t(($) => $.telegramEditor.textLabel)}
          description={t(($) => $.telegramEditor.textHint)}
          autosize
          minRows={4}
          maxRows={10}
          value={value.text}
          error={
            over
              ? t(($) => $.telegramEditor.tooLong, {
                  limit,
                  what: t(($) =>
                    value.imageUrl.trim() === ''
                      ? $.telegramEditor.whatText
                      : $.telegramEditor.whatCaption,
                  ),
                })
              : undefined
          }
          onChange={(e) => patch({ text: e.currentTarget.value })}
        />
        <Group justify="space-between">
          <Menu>
            <Menu.Target>
              <Text component="button" type="button" size="sm" c="blue">
                {t(($) => $.telegramEditor.insertMergeTag)}
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
          <Text size="sm" c={over ? 'red' : 'dimmed'}>
            {t(($) => $.telegramEditor.counter, { length: value.text.length, limit })}
          </Text>
        </Group>
      </Stack>

      <TextInput
        label={t(($) => $.telegramEditor.imageLabel)}
        description={t(($) => $.telegramEditor.imageHint)}
        placeholder="https://"
        value={value.imageUrl}
        onChange={(e) => patch({ imageUrl: e.currentTarget.value })}
      />

      <Stack gap="xs">
        <Text size="sm" fw={500}>
          {t(($) => $.telegramEditor.buttonsLabel)}
        </Text>
        {value.buttons.map((button, i) => (
          <Group key={i} gap="xs" align="flex-end" wrap="nowrap">
            <TextInput
              aria-label={t(($) => $.telegramEditor.buttonLabel, { n: i + 1 })}
              placeholder={t(($) => $.telegramEditor.buttonLabelPlaceholder)}
              value={button.label}
              onChange={(e) => setButton(i, { label: e.currentTarget.value })}
            />
            <TextInput
              aria-label={t(($) => $.telegramEditor.buttonUrl, { n: i + 1 })}
              placeholder="https://"
              value={button.url}
              onChange={(e) => setButton(i, { url: e.currentTarget.value })}
            />
            <ActionIcon
              variant="subtle"
              color="red"
              aria-label={t(($) => $.telegramEditor.removeButton, { n: i + 1 })}
              onClick={() => patch({ buttons: value.buttons.filter((_, j) => j !== i) })}
            >
              <IconTrash size={16} />
            </ActionIcon>
          </Group>
        ))}
        <Button
          variant="default"
          size="xs"
          disabled={value.buttons.length >= TELEGRAM_MAX_BUTTONS}
          onClick={() => patch({ buttons: [...value.buttons, { label: '', url: '' }] })}
        >
          {t(($) => $.telegramEditor.addButton)}
        </Button>
      </Stack>

      <Switch
        label={t(($) => $.telegramEditor.linkPreview)}
        checked={value.linkPreview}
        onChange={(e) => patch({ linkPreview: e.currentTarget.checked })}
      />
      {children}

      <Stack gap="xs">
        <Text size="sm" fw={500}>
          {t(($) => $.telegramEditor.previewTitle)}
        </Text>
        <TelegramPreview content={value} />
      </Stack>
    </Stack>
  )
}
