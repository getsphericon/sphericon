import { Group, TextInput } from '@mantine/core'
import { MantineValueSelector } from '@react-querybuilder/mantine'
import { useTranslation } from 'react-i18next'
import type { FieldSelectorProps } from 'react-querybuilder'

import { triggerFieldPrefix } from './triggerFieldPrefix.ts'

// TriggerFieldSelector is the builder's field picker with a key input beside the "Event
// property" choice: the picker holds the bare prefix, the input the key after it.
export function TriggerFieldSelector(props: FieldSelectorProps) {
  const { t } = useTranslation()
  const value = props.value ?? ''
  const isTrigger = value.startsWith(triggerFieldPrefix)
  return (
    <Group gap="xs" wrap="nowrap">
      <MantineValueSelector {...props} value={isTrigger ? triggerFieldPrefix : value} />
      {isTrigger ? (
        <TextInput
          aria-label={t(($) => $.segments.triggerKeyLabel)}
          placeholder={t(($) => $.segments.triggerKeyLabel)}
          value={value.slice(triggerFieldPrefix.length)}
          onChange={(event) => props.handleOnChange(triggerFieldPrefix + event.currentTarget.value)}
        />
      ) : null}
    </Group>
  )
}
