import { Group, Select, TextInput } from '@mantine/core'
import type { useForm } from '@mantine/form'
import { useTranslation } from 'react-i18next'

// Name + trigger are the automation's metadata, edited as plain Mantine inputs
// alongside the visual builder (the canvas owns the steps / definition).
export interface AutomationDetailsValues {
  name: string
  triggerEvent: string
}

type AutomationDetailsForm = ReturnType<typeof useForm<AutomationDetailsValues>>

export function AutomationDetailsFields({ form }: { form: AutomationDetailsForm }) {
  const { t } = useTranslation()

  const triggerOptions = [
    { value: 'contact.created', label: t(($) => $.automations.triggerContactCreated) },
    { value: 'email.opened', label: t(($) => $.automations.triggerEmailOpened) },
    { value: 'email.clicked', label: t(($) => $.automations.triggerEmailClicked) },
    { value: 'email.unsubscribed', label: t(($) => $.automations.triggerEmailUnsubscribed) },
  ]

  return (
    <Group align="flex-start" grow>
      <TextInput
        label={t(($) => $.automations.nameLabel)}
        required
        {...form.getInputProps('name')}
      />
      <Select
        label={t(($) => $.automations.triggerLabel)}
        description={t(($) => $.automations.triggerHint)}
        data={triggerOptions}
        required
        allowDeselect={false}
        {...form.getInputProps('triggerEvent')}
      />
    </Group>
  )
}
