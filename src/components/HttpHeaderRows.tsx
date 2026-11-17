import { ActionIcon, Button, Group, Stack, TextInput } from '@mantine/core'
import type { GetInputPropsReturnType } from '@mantine/form'
import { IconTrash } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import type { AutomationHttpHeader } from '../generated/site/types.gen.ts'
import { zRuleAction } from '../generated/site/zod.gen.ts'
import { maxItemsOf } from '../utils/bounds.ts'
import { PasswordField } from './PasswordField.tsx'

// The most request headers an outgoing HTTP request carries, as the contract declares it: a
// Rule's http Action and an Automation's HTTP step alike.
const HTTP_HEADERS_LIMIT = maxItemsOf(zRuleAction.shape.headers.unwrap())

interface HttpHeaderRowsProps {
  // The headers as edited: the server returns only the name of a stored header, so a value left
  // empty keeps the stored one.
  headers: AutomationHttpHeader[]
  // Whether the headers were saved before: a header with no value then keeps its stored one.
  saved: boolean
  onChange: (headers: AutomationHttpHeader[]) => void
  // In a form: the form's props of a header's field (`0.name`, `0.value`), which bring its error
  // and its path, so a refusal shows under the field and a failed submit focuses it.
  inputProps?: (field: string) => GetInputPropsReturnType
}

// The request headers of an outgoing HTTP call as rows of name and value, added up to the limit
// and removed one by one. A value is a secret (an API key, a bearer token), so it is masked.
export function HttpHeaderRows({ headers, saved, onChange, inputProps }: HttpHeaderRowsProps) {
  const { t } = useTranslation()
  const change = (index: number, edit: Partial<AutomationHttpHeader>) =>
    onChange(headers.map((header, at) => (at === index ? { ...header, ...edit } : header)))

  return (
    <Stack gap="xs">
      {headers.map((header, index) => (
        <Group key={index} align="flex-end" wrap="nowrap">
          <TextInput
            label={t(($) => $.httpHeaders.nameLabel)}
            onChange={(event) => change(index, { name: event.currentTarget.value })}
            {...inputProps?.(`${index}.name`)}
            value={header.name}
          />
          <PasswordField
            label={t(($) => $.httpHeaders.valueLabel)}
            description={saved && !header.value ? t(($) => $.httpHeaders.keepHint) : undefined}
            onChange={(event) => change(index, { value: event.currentTarget.value })}
            {...inputProps?.(`${index}.value`)}
            value={header.value ?? ''}
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
