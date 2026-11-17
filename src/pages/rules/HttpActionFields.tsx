import { Stack, Textarea, TextInput } from '@mantine/core'
import type { GetInputPropsReturnType } from '@mantine/form'
import { useTranslation } from 'react-i18next'

import { HttpHeaderRows } from '../../components/HttpHeaderRows.tsx'
import { PasswordField } from '../../components/PasswordField.tsx'
import type { RuleAction } from '../../generated/site/types.gen.ts'

interface HttpActionFieldsProps {
  action: RuleAction
  // The form's props of a field of the Action (`url`, `headers.0.name`): its value, its change,
  // its error and its path.
  inputProps: (field: string) => GetInputPropsReturnType
}

// The fields of an http Action. The server never returns the URL, the header values or the
// signing secret: an Action that already has an id leaves them blank to keep what is stored.
export function HttpActionFields({ action, inputProps }: HttpActionFieldsProps) {
  const { t } = useTranslation()
  const saved = Boolean(action.id)
  const headers = inputProps('headers')
  return (
    <Stack gap="xs">
      <TextInput
        label={t(($) => $.rules.http.urlLabel)}
        description={saved ? t(($) => $.rules.http.keepHint) : undefined}
        {...inputProps('url')}
        value={action.url ?? ''}
      />
      <HttpHeaderRows
        headers={action.headers ?? []}
        saved={saved}
        onChange={headers.onChange}
        inputProps={(field) => inputProps(`headers.${field}`)}
      />
      <PasswordField
        label={t(($) => $.rules.http.secretLabel)}
        description={
          action.hasSecret ? t(($) => $.rules.http.keepHint) : t(($) => $.rules.http.secretHint)
        }
        {...inputProps('secret')}
        value={action.secret ?? ''}
      />
      <Textarea
        label={t(($) => $.rules.http.bodyLabel)}
        description={t(($) => $.rules.http.bodyHint)}
        autosize
        minRows={3}
        {...inputProps('body')}
        value={action.body ?? ''}
      />
    </Stack>
  )
}
