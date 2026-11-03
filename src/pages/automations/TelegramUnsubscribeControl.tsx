import { Stack, Switch, TextInput } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { TELEGRAM_UNSUBSCRIBE_LIMIT } from './telegramFormat.ts'

// TelegramUnsubscribeControl edits the label of a Telegram step's unsubscribe button: a switch adds
// the button (its label starts as a default) and an input names it. The value is the label, '' for no
// button, as the API holds it. It sits in the shared Telegram editor's `children` slot.
export function TelegramUnsubscribeControl({
  label,
  onChange,
}: {
  label: string
  onChange: (label: string) => void
}) {
  const { t } = useTranslation()
  const on = label !== ''

  return (
    <Stack gap="xs">
      <Switch
        label={t(($) => $.automations.telegramUnsubscribeToggle)}
        description={t(($) => $.automations.telegramUnsubscribeHint)}
        checked={on}
        onChange={(e) =>
          onChange(
            e.currentTarget.checked ? t(($) => $.automations.telegramUnsubscribeDefault) : '',
          )
        }
      />
      {on && (
        <TextInput
          label={t(($) => $.automations.telegramUnsubscribeLabel)}
          maxLength={TELEGRAM_UNSUBSCRIBE_LIMIT}
          value={label}
          onChange={(e) => onChange(e.currentTarget.value)}
        />
      )}
    </Stack>
  )
}
