import { ActionIcon, Button, Group, PasswordInput, Stack, Textarea, TextInput } from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import type { SiteRuleAction } from '../../generated/site/types.gen.ts'

// The most request headers an http Action carries (the server refuses more).
const MAX_HEADERS = 5

interface HttpActionFieldsProps {
  action: SiteRuleAction
  onChange: (change: Partial<SiteRuleAction>) => void
}

// The fields of an http Action. The server never returns the URL, the header values or the
// signing secret: an Action that already has an id leaves them blank to keep what is stored.
export function HttpActionFields({ action, onChange }: HttpActionFieldsProps) {
  const { t } = useTranslation()
  const saved = Boolean(action.id)
  const headers = action.headers ?? []
  const changeHeader = (index: number, change: { name?: string; value?: string }) =>
    onChange({
      headers: headers.map((header, at) => (at === index ? { ...header, ...change } : header)),
    })

  return (
    <Stack gap="xs">
      <TextInput
        label={t(($) => $.rules.http.urlLabel)}
        description={saved ? t(($) => $.rules.http.keepHint) : undefined}
        value={action.url ?? ''}
        onChange={(event) => onChange({ url: event.currentTarget.value })}
      />
      {headers.map((header, index) => (
        <Group key={index} align="flex-end" wrap="nowrap">
          <TextInput
            label={t(($) => $.rules.http.headerNameLabel)}
            value={header.name}
            onChange={(event) => changeHeader(index, { name: event.currentTarget.value })}
          />
          <PasswordInput
            label={t(($) => $.rules.http.headerValueLabel)}
            description={saved && !header.value ? t(($) => $.rules.http.keepHint) : undefined}
            value={header.value ?? ''}
            onChange={(event) => changeHeader(index, { value: event.currentTarget.value })}
          />
          <ActionIcon
            variant="subtle"
            color="red"
            aria-label={t(($) => $.rules.http.removeHeader)}
            onClick={() => onChange({ headers: headers.filter((_, at) => at !== index) })}
          >
            <IconTrash size={16} />
          </ActionIcon>
        </Group>
      ))}
      {headers.length < MAX_HEADERS ? (
        <Group>
          <Button
            variant="light"
            size="compact-sm"
            onClick={() => onChange({ headers: [...headers, { name: '' }] })}
          >
            {t(($) => $.rules.http.addHeader)}
          </Button>
        </Group>
      ) : null}
      <PasswordInput
        label={t(($) => $.rules.http.secretLabel)}
        description={
          action.hasSecret ? t(($) => $.rules.http.keepHint) : t(($) => $.rules.http.secretHint)
        }
        value={action.secret ?? ''}
        onChange={(event) => onChange({ secret: event.currentTarget.value })}
      />
      <Textarea
        label={t(($) => $.rules.http.bodyLabel)}
        description={t(($) => $.rules.http.bodyHint)}
        autosize
        minRows={3}
        value={action.body ?? ''}
        onChange={(event) => onChange({ body: event.currentTarget.value })}
      />
    </Stack>
  )
}
