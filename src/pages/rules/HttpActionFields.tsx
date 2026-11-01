import { PasswordInput, Stack, Textarea, TextInput } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { HttpHeaderRows } from '../../components/HttpHeaderRows.tsx'
import type { SiteRuleAction } from '../../generated/site/types.gen.ts'

interface HttpActionFieldsProps {
  action: SiteRuleAction
  onChange: (change: Partial<SiteRuleAction>) => void
}

// The fields of an http Action. The server never returns the URL, the header values or the
// signing secret: an Action that already has an id leaves them blank to keep what is stored.
export function HttpActionFields({ action, onChange }: HttpActionFieldsProps) {
  const { t } = useTranslation()
  const saved = Boolean(action.id)
  return (
    <Stack gap="xs">
      <TextInput
        label={t(($) => $.rules.http.urlLabel)}
        description={saved ? t(($) => $.rules.http.keepHint) : undefined}
        value={action.url ?? ''}
        onChange={(event) => onChange({ url: event.currentTarget.value })}
      />
      <HttpHeaderRows
        headers={action.headers ?? []}
        saved={saved}
        onChange={(headers) => onChange({ headers })}
      />
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
