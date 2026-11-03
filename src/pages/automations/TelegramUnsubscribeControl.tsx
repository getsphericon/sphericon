import { Stack, Switch, TextInput } from '@mantine/core'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useTranslation } from 'react-i18next'

import { TELEGRAM_UNSUBSCRIBE_LIMIT } from './telegramFormat.ts'

// TelegramUnsubscribeControl edits the label of a Telegram step's unsubscribe button: a switch adds
// the button (its label starts as a default) and an input names it. The value is the label, '' for no
// button, as the API holds it.
export function TelegramUnsubscribeControl(props: ControlProps) {
  const { data, path } = props
  const { t } = useTranslation()
  const label = typeof data === 'string' ? data : ''
  const on = label !== ''

  return (
    <Stack gap="xs">
      <Switch
        label={t(($) => $.automations.telegramUnsubscribeToggle)}
        description={t(($) => $.automations.telegramUnsubscribeHint)}
        checked={on}
        onChange={(e) =>
          props.handleChange(
            path,
            e.currentTarget.checked ? t(($) => $.automations.telegramUnsubscribeDefault) : '',
          )
        }
      />
      {on && (
        <TextInput
          label={t(($) => $.automations.telegramUnsubscribeLabel)}
          maxLength={TELEGRAM_UNSUBSCRIBE_LIMIT}
          value={label}
          onChange={(e) => props.handleChange(path, e.currentTarget.value)}
        />
      )}
    </Stack>
  )
}
