import { ActionIcon, Button, Group, PasswordInput, Stack, TextInput } from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import type { SiteAutomationHttpHeader } from '../generated/site/types.gen.ts'
import { zSiteRuleAction } from '../generated/site/zod.gen.ts'
import { maxItemsOf } from '../utils/bounds.ts'

// The most request headers an outgoing HTTP request carries, as the contract declares it: a
// Rule's http Action and an Automation's HTTP step alike.
const HTTP_HEADERS_LIMIT = maxItemsOf(zSiteRuleAction.shape.headers.unwrap())

interface HttpHeaderRowsProps {
  // The headers as edited: the server returns only the name of a stored header, so a value left
  // empty keeps the stored one.
  headers: SiteAutomationHttpHeader[]
  // Whether the headers were saved before: a header with no value then keeps its stored one.
  saved: boolean
  onChange: (headers: SiteAutomationHttpHeader[]) => void
}

// The request headers of an outgoing HTTP call as rows of name and value, added up to the limit
// and removed one by one. A value is a secret (an API key, a bearer token), so it is masked.
export function HttpHeaderRows({ headers, saved, onChange }: HttpHeaderRowsProps) {
  const { t } = useTranslation()
  const change = (index: number, edit: Partial<SiteAutomationHttpHeader>) =>
    onChange(headers.map((header, at) => (at === index ? { ...header, ...edit } : header)))

  return (
    <Stack gap="xs">
      {headers.map((header, index) => (
        <Group key={index} align="flex-end" wrap="nowrap">
          <TextInput
            label={t(($) => $.httpHeaders.nameLabel)}
            value={header.name}
            onChange={(event) => change(index, { name: event.currentTarget.value })}
          />
          <PasswordInput
            label={t(($) => $.httpHeaders.valueLabel)}
            description={saved && !header.value ? t(($) => $.httpHeaders.keepHint) : undefined}
            value={header.value ?? ''}
            onChange={(event) => change(index, { value: event.currentTarget.value })}
          />
          <ActionIcon
            variant="subtle"
            color="red"
            aria-label={t(($) => $.httpHeaders.remove)}
            onClick={() => onChange(headers.filter((_, at) => at !== index))}
          >
            <IconTrash size={16} />
          </ActionIcon>
        </Group>
      ))}
      {headers.length < HTTP_HEADERS_LIMIT ? (
        <Group>
          <Button
            variant="light"
            size="compact-sm"
            onClick={() => onChange([...headers, { name: '' }])}
          >
            {t(($) => $.httpHeaders.add)}
          </Button>
        </Group>
      ) : null}
    </Stack>
  )
}
