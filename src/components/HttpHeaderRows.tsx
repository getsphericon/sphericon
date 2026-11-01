import { ActionIcon, Button, Group, PasswordInput, Stack, TextInput } from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

// The most request headers an outgoing HTTP request carries: a Rule's http Action and an
// Automation's HTTP step alike (the server refuses more).
const MAX_HTTP_HEADERS = 5

// One request header as edited: the server returns only the name of a stored header, so a value
// left empty keeps the stored one.
export interface HttpHeader {
  name: string
  value?: string
}

interface HttpHeaderRowsProps {
  headers: HttpHeader[]
  // Whether the headers were saved before: a header with no value then keeps its stored one.
  saved: boolean
  onChange: (headers: HttpHeader[]) => void
}

// The request headers of an outgoing HTTP call as rows of name and value, added up to the limit
// and removed one by one. A value is a secret (an API key, a bearer token), so it is masked.
export function HttpHeaderRows({ headers, saved, onChange }: HttpHeaderRowsProps) {
  const { t } = useTranslation()
  const change = (index: number, edit: Partial<HttpHeader>) =>
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
      {headers.length < MAX_HTTP_HEADERS ? (
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
